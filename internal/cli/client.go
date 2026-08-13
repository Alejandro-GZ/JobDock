package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

type APIError struct {
	Status        int
	Code, Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func (c *Client) Do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	base, err := url.Parse(strings.TrimRight(c.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}
	rel, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	var source io.Reader
	if body != nil {
		data, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return nil, marshalErr
		}
		source = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, base.ResolveReference(rel).String(), source)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("Idempotency-Key", fmt.Sprintf("cli-%d", time.Now().UnixNano()))
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		var problem struct {
			Code    string `json:"code"`
			Detail  string `json:"detail"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&problem)
		if problem.Code == "" {
			problem.Code = "http_error"
		}
		if problem.Detail != "" {
			problem.Message = problem.Detail
		}
		if problem.Message == "" {
			problem.Message = resp.Status
		}
		return nil, &APIError{Status: resp.StatusCode, Code: problem.Code, Message: problem.Message}
	}
	return resp, nil
}

func decode[T any](resp *http.Response) (T, error) {
	defer resp.Body.Close()
	var result T
	err := json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}
