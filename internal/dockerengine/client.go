package dockerengine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	http *http.Client
}

type Info struct {
	NCPU            int    `json:"NCPU"`
	MemTotal        int64  `json:"MemTotal"`
	Name            string `json:"Name"`
	Architecture    string `json:"Architecture"`
	ServerVersion   string `json:"ServerVersion"`
	OperatingSystem string `json:"OperatingSystem"`
	OSVersion       string `json:"OSVersion"`
	OSType          string `json:"OSType"`
	KernelVersion   string `json:"KernelVersion"`
	Driver          string `json:"Driver"`
	CgroupDriver    string `json:"CgroupDriver"`
	CgroupVersion   string `json:"CgroupVersion"`
}

type ResourceStats struct {
	CPUMillis   int64
	MemoryBytes int64
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
}

type Container struct {
	ID     string            `json:"Id"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

type CreateOptions struct {
	Name             string
	JobID            string
	AttemptID        string
	Image            string
	Command          []string
	WorkingDirectory string
	Environment      []string
	Binds            []string
	CPUMillis        int64
	CPUSet           string
	MemoryBytes      int64
	GPUUUIDs         []string
}

type deviceRequest struct {
	Driver       string     `json:"Driver"`
	DeviceIDs    []string   `json:"DeviceIDs"`
	Capabilities [][]string `json:"Capabilities"`
}

type hostConfig struct {
	Binds          []string        `json:"Binds"`
	Memory         int64           `json:"Memory"`
	NanoCPUs       int64           `json:"NanoCpus"`
	CpusetCpus     string          `json:"CpusetCpus,omitempty"`
	PidsLimit      int64           `json:"PidsLimit"`
	CapDrop        []string        `json:"CapDrop"`
	SecurityOpt    []string        `json:"SecurityOpt"`
	DeviceRequests []deviceRequest `json:"DeviceRequests,omitempty"`
}

type createContainerRequest struct {
	Image      string            `json:"Image"`
	Cmd        []string          `json:"Cmd"`
	Env        []string          `json:"Env"`
	WorkingDir string            `json:"WorkingDir,omitempty"`
	Labels     map[string]string `json:"Labels"`
	HostConfig hostConfig        `json:"HostConfig"`
}

func createRequest(options CreateOptions) createContainerRequest {
	body := createContainerRequest{Image: options.Image, Cmd: options.Command, Env: options.Environment, WorkingDir: options.WorkingDirectory, Labels: map[string]string{"jobdock.managed": "true", "jobdock.job_id": options.JobID, "jobdock.attempt_id": options.AttemptID}, HostConfig: hostConfig{Binds: options.Binds, Memory: options.MemoryBytes, NanoCPUs: options.CPUMillis * 1_000_000, CpusetCpus: options.CPUSet, PidsLimit: 1024, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"}}}
	if len(options.GPUUUIDs) > 0 {
		body.HostConfig.DeviceRequests = []deviceRequest{{Driver: "nvidia", DeviceIDs: options.GPUUUIDs, Capabilities: [][]string{{"gpu"}}}}
	}
	return body
}

func New(socket string) *Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "unix", socket)
	}, MaxIdleConns: 16, IdleConnTimeout: 90 * time.Second}
	return &Client{http: &http.Client{Transport: transport}}
}

func (c *Client) Ping(ctx context.Context) error {
	response, err := c.request(ctx, "GET", "/_ping", nil, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return apiError(response)
	}
	return nil
}
func (c *Client) Info(ctx context.Context) (Info, error) {
	var info Info
	response, err := c.request(ctx, "GET", "/info", nil, nil)
	if err != nil {
		return info, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return info, apiError(response)
	}
	return info, json.NewDecoder(response.Body).Decode(&info)
}

func (c *Client) Pull(ctx context.Context, image string, registryAuth string) error {
	headers := http.Header{}
	if registryAuth != "" {
		headers.Set("X-Registry-Auth", registryAuth)
	}
	response, err := c.request(ctx, "POST", "/images/create?fromImage="+url.QueryEscape(image), nil, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return apiError(response)
	}
	scanner := bufio.NewScanner(response.Body)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 2<<20)
	for scanner.Scan() {
		var item struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &item) == nil && item.Error != "" {
			return errors.New(item.Error)
		}
	}
	return scanner.Err()
}

func (c *Client) Load(ctx context.Context, archive io.Reader) error {
	response, err := c.request(ctx, "POST", "/images/load?quiet=1", archive, http.Header{"Content-Type": []string{"application/x-tar"}})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return apiError(response)
	}
	scanner := bufio.NewScanner(response.Body)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 2<<20)
	for scanner.Scan() {
		var item struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &item) == nil && item.Error != "" {
			return errors.New(item.Error)
		}
	}
	return scanner.Err()
}

func (c *Client) ImageDigest(ctx context.Context, image string) string {
	response, err := c.request(ctx, "GET", "/images/"+url.PathEscape(image)+"/json", nil, nil)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return ""
	}
	var data struct {
		RepoDigests []string `json:"RepoDigests"`
		ID          string   `json:"Id"`
	}
	if json.NewDecoder(response.Body).Decode(&data) != nil {
		return ""
	}
	if len(data.RepoDigests) > 0 {
		return data.RepoDigests[0]
	}
	return data.ID
}

func (c *Client) Create(ctx context.Context, options CreateOptions) (string, error) {
	body := createRequest(options)
	data, _ := json.Marshal(body)
	response, err := c.request(ctx, "POST", "/containers/create?name="+url.QueryEscape(options.Name), bytes.NewReader(data), jsonHeader())
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		return "", apiError(response)
	}
	var result struct {
		ID string `json:"Id"`
	}
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ID, nil
}

func (c *Client) Start(ctx context.Context, id string) error {
	response, err := c.request(ctx, "POST", "/containers/"+id+"/start", nil, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 204 && response.StatusCode != 304 {
		return apiError(response)
	}
	return nil
}
func (c *Client) Stop(ctx context.Context, id string, graceSeconds int) error {
	response, err := c.request(ctx, "POST", fmt.Sprintf("/containers/%s/stop?t=%d", id, graceSeconds), nil, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 204 && response.StatusCode != 304 && response.StatusCode != 404 {
		return apiError(response)
	}
	return nil
}
func (c *Client) Remove(ctx context.Context, id string) error {
	response, err := c.request(ctx, "DELETE", "/containers/"+id+"?force=1", nil, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 204 && response.StatusCode != 404 {
		return apiError(response)
	}
	return nil
}

func (c *Client) Wait(ctx context.Context, id string) (int, error) {
	response, err := c.request(ctx, "POST", "/containers/"+id+"/wait?condition=not-running", nil, nil)
	if err != nil {
		return -1, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return -1, apiError(response)
	}
	var result struct {
		StatusCode int `json:"StatusCode"`
		Error      *struct {
			Message string `json:"Message"`
		} `json:"Error"`
	}
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		return -1, err
	}
	if result.Error != nil && result.Error.Message != "" {
		return result.StatusCode, errors.New(result.Error.Message)
	}
	return result.StatusCode, nil
}

func (c *Client) Logs(ctx context.Context, id string, callback func(stream string, payload []byte) error) error {
	response, err := c.request(ctx, "GET", "/containers/"+id+"/logs?follow=1&stdout=1&stderr=1&timestamps=0", nil, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return apiError(response)
	}
	header := make([]byte, 8)
	for {
		if _, err = io.ReadFull(response.Body, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		size := binary.BigEndian.Uint32(header[4:8])
		if size > 16<<20 {
			return errors.New("docker log frame exceeds 16 MiB")
		}
		payload := make([]byte, size)
		if _, err = io.ReadFull(response.Body, payload); err != nil {
			return err
		}
		stream := "stdout"
		if header[0] == 2 {
			stream = "stderr"
		}
		if err = callback(stream, payload); err != nil {
			return err
		}
	}
}

func (c *Client) ManagedContainers(ctx context.Context) ([]Container, error) {
	filters := url.QueryEscape(`{"label":["jobdock.managed=true"]}`)
	response, err := c.request(ctx, "GET", "/containers/json?all=1&filters="+filters, nil, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, apiError(response)
	}
	var containers []Container
	return containers, json.NewDecoder(response.Body).Decode(&containers)
}

func (c *Client) Stats(ctx context.Context, id string) (ResourceStats, error) {
	response, err := c.request(ctx, "GET", "/containers/"+id+"/stats?stream=false", nil, nil)
	if err != nil {
		return ResourceStats{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return ResourceStats{}, apiError(response)
	}
	var raw dockerStats
	if err = json.NewDecoder(response.Body).Decode(&raw); err != nil {
		return ResourceStats{}, err
	}
	return normalizeStats(raw), nil
}

func normalizeStats(raw dockerStats) ResourceStats {
	result := ResourceStats{}
	cores := raw.CPUStats.OnlineCPUs
	if cores == 0 {
		cores = len(raw.CPUStats.CPUUsage.PercpuUsage)
	}
	if cores > 0 && raw.CPUStats.CPUUsage.TotalUsage >= raw.PreCPUStats.CPUUsage.TotalUsage && raw.CPUStats.SystemCPUUsage > raw.PreCPUStats.SystemCPUUsage {
		cpuDelta := raw.CPUStats.CPUUsage.TotalUsage - raw.PreCPUStats.CPUUsage.TotalUsage
		systemDelta := raw.CPUStats.SystemCPUUsage - raw.PreCPUStats.SystemCPUUsage
		result.CPUMillis = int64(float64(cpuDelta)/float64(systemDelta)*float64(cores)*1000 + 0.5)
	}
	memory := raw.MemoryStats.Usage
	cache := raw.MemoryStats.Stats["inactive_file"]
	if cache == 0 {
		cache = raw.MemoryStats.Stats["total_inactive_file"]
	}
	if cache <= memory {
		memory -= cache
	}
	result.MemoryBytes = int64(memory)
	return result
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader, headers http.Header) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, body)
	if err != nil {
		return nil, err
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	return c.http.Do(request)
}
func jsonHeader() http.Header {
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	return header
}
func apiError(response *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	var item struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &item)
	if item.Message == "" {
		item.Message = strings.TrimSpace(string(data))
	}
	return fmt.Errorf("docker API %s: %s", response.Status, item.Message)
}
