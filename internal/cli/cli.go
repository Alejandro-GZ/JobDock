package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitAuth     = 3
	ExitNotFound = 4
	ExitConflict = 5
)

type App struct {
	Out, Err io.Writer
	HTTP     *http.Client
	Version  string
	getenv   func(string) string
}

func New(out, err io.Writer) *App { return &App{Out: out, Err: err, Version: "dev", getenv: os.Getenv} }

func (a *App) WithVersion(version string) *App {
	if strings.TrimSpace(version) != "" {
		a.Version = version
	}
	return a
}

func (a *App) Run(ctx context.Context, args []string) int {
	global := flag.NewFlagSet("jobdock", flag.ContinueOnError)
	global.SetOutput(a.Err)
	server := global.String("server", a.env("JOBDOCK_URL", "http://localhost:8080"), "JobDock server URL")
	tokenFile := global.String("token-file", a.env("JOBDOCK_TOKEN_FILE", ""), "file containing a personal access token")
	format := global.String("format", "human", "output format: human or json")
	showVersion := global.Bool("version", false, "print the CLI version and server compatibility")
	global.Usage = func() {
		fmt.Fprintln(a.Err, "Usage: jobdock [--server URL] [--token-file PATH] [--format human|json] <version|nodes|jobs|run|logs|stop|download> ...")
	}
	if err := global.Parse(args); err != nil {
		return ExitUsage
	}
	if *showVersion {
		fmt.Fprintf(a.Out, "jobdock %s (server API v1)\n", a.Version)
		return ExitOK
	}
	remaining := global.Args()
	if len(remaining) == 0 {
		global.Usage()
		return ExitUsage
	}
	if *format != "human" && *format != "json" {
		return a.fail(*format, ExitUsage, "invalid_output_format", "format must be human or json")
	}
	if remaining[0] == "version" {
		if *format == "json" {
			_ = writeJSON(a.Out, map[string]string{"version": a.Version, "server_api": "v1"})
		} else {
			fmt.Fprintf(a.Out, "jobdock %s (server API v1)\n", a.Version)
		}
		return ExitOK
	}
	token := strings.TrimSpace(a.getenv("JOBDOCK_TOKEN"))
	if *tokenFile != "" {
		data, err := os.ReadFile(*tokenFile)
		if err != nil {
			return a.fail(*format, ExitUsage, "token_file_error", err.Error())
		}
		token = strings.TrimSpace(string(data))
	}
	if token == "" {
		return a.fail(*format, ExitAuth, "authentication_required", "set JOBDOCK_TOKEN or --token-file")
	}
	client := &Client{BaseURL: *server, Token: token, HTTP: a.HTTP}
	var err error
	switch remaining[0] {
	case "nodes":
		err = a.nodes(ctx, client, *format)
	case "jobs":
		err = a.jobs(ctx, client, *format)
	case "run":
		err = a.run(ctx, client, *format, remaining[1:])
	case "logs":
		err = a.logs(ctx, client, *format, remaining[1:])
	case "stop":
		err = a.stop(ctx, client, *format, remaining[1:])
	case "download":
		err = a.download(ctx, client, *format, remaining[1:])
	case "help", "--help", "-h":
		global.Usage()
		return ExitOK
	default:
		return a.fail(*format, ExitUsage, "unknown_command", "unknown command: "+remaining[0])
	}
	if err == nil {
		return ExitOK
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		code := ExitError
		switch apiErr.Status {
		case 401, 403:
			code = ExitAuth
		case 404:
			code = ExitNotFound
		case 409:
			code = ExitConflict
		}
		return a.fail(*format, code, apiErr.Code, apiErr.Message)
	}
	return a.fail(*format, ExitError, "command_failed", err.Error())
}

func (a *App) nodes(ctx context.Context, client *Client, format string) error {
	resp, err := client.Do(ctx, http.MethodGet, "/api/v1/nodes", nil)
	if err != nil {
		return err
	}
	result, err := decode[struct {
		Items []domain.Node `json:"items"`
	}](resp)
	if err != nil {
		return err
	}
	if format == "json" {
		return writeJSON(a.Out, result.Items)
	}
	w := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tCPU\tMEMORY\tGPUS")
	for _, n := range result.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d/%d mCPU\t%d/%d bytes\t%d\n", n.ID, n.Name, n.Status, n.CPUAllocatedMillis, n.CPUTotalMillis, n.MemoryAllocatedBytes, n.MemoryTotalBytes, len(n.GPUs))
	}
	return w.Flush()
}

func (a *App) jobs(ctx context.Context, client *Client, format string) error {
	resp, err := client.Do(ctx, http.MethodGet, "/api/v1/jobs", nil)
	if err != nil {
		return err
	}
	result, err := decode[struct {
		Items []domain.Job `json:"items"`
	}](resp)
	if err != nil {
		return err
	}
	if format == "json" {
		return writeJSON(a.Out, result.Items)
	}
	w := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tNODE\tCREATED")
	for _, j := range result.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", j.ID, j.Spec.Name, j.Status, j.AssignedNodeID, j.CreatedAt.Format(time.RFC3339))
	}
	return w.Flush()
}

type valuesFlag []string

func (v *valuesFlag) String() string     { return strings.Join(*v, ",") }
func (v *valuesFlag) Set(s string) error { *v = append(*v, s); return nil }

func (a *App) run(ctx context.Context, client *Client, format string, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	name, image := fs.String("name", "", "job name"), fs.String("image", "", "OCI image")
	cpu, memory, gpus, vram := fs.Int64("cpu", 1000, "CPU millicores"), fs.Int64("memory", 512<<20, "memory bytes"), fs.Int("gpus", 0, "GPU count"), fs.Int64("min-vram", 0, "minimum GPU VRAM bytes")
	working := fs.String("working-directory", "", "container working directory")
	var env, labels, selectors valuesFlag
	fs.Var(&env, "env", "environment KEY=VALUE (repeatable)")
	fs.Var(&labels, "label", "label KEY=VALUE (repeatable)")
	fs.Var(&selectors, "selector", "node selector KEY=VALUE (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" || *image == "" || len(fs.Args()) == 0 {
		return fmt.Errorf("run requires --name, --image and a command after --")
	}
	spec := domain.JobSpec{Name: *name, Image: *image, Command: fs.Args(), WorkingDirectory: *working, Resources: domain.Resources{CPUMillis: *cpu, MemoryBytes: *memory, GPU: domain.GPURequest{Count: *gpus, MinVRAMBytes: *vram}}}
	var err error
	if spec.Environment, err = parsePairs(env); err != nil {
		return err
	}
	if spec.Labels, err = parsePairs(labels); err != nil {
		return err
	}
	if spec.NodeSelector, err = parsePairs(selectors); err != nil {
		return err
	}
	resp, err := client.Do(ctx, http.MethodPost, "/api/v1/jobs", spec)
	if err != nil {
		return err
	}
	job, err := decode[domain.Job](resp)
	if err != nil {
		return err
	}
	if format == "json" {
		return writeJSON(a.Out, job)
	}
	fmt.Fprintln(a.Out, job.ID)
	return nil
}

func (a *App) logs(ctx context.Context, client *Client, format string, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	follow := fs.Bool("f", false, "follow logs")
	stream := fs.String("stream", "stdout", "stdout or stderr")
	attempt := fs.String("attempt", "", "attempt ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("logs requires one job ID")
	}
	if *stream != "stdout" && *stream != "stderr" {
		return fmt.Errorf("stream must be stdout or stderr")
	}
	if format == "json" && *follow {
		return fmt.Errorf("JSON output is not supported with --follow")
	}
	offset := int64(0)
	for {
		path := fmt.Sprintf("/api/v1/jobs/%s/logs/%s?offset=%d", fs.Arg(0), *stream, offset)
		if *attempt != "" {
			path += "&attempt_id=" + *attempt
		}
		resp, err := client.Do(ctx, http.MethodGet, path, nil)
		if err != nil {
			if *follow && ctx.Err() != nil {
				return nil
			}
			return err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		next, _ := strconv.ParseInt(resp.Header.Get("X-JobDock-Next-Offset"), 10, 64)
		offset = next
		if format == "json" {
			if err = writeJSON(a.Out, map[string]any{"stream": *stream, "offset": offset, "data": string(data)}); err != nil {
				return err
			}
		} else if _, err = a.Out.Write(data); err != nil {
			return err
		}
		if !*follow {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func (a *App) stop(ctx context.Context, client *Client, format string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("stop requires one job ID")
	}
	resp, err := client.Do(ctx, http.MethodPost, "/api/v1/jobs/"+args[0]+"/stop", map[string]any{})
	if err != nil {
		return err
	}
	result, err := decode[map[string]string](resp)
	if err != nil {
		return err
	}
	if format == "json" {
		return writeJSON(a.Out, result)
	}
	fmt.Fprintln(a.Out, result["status"])
	return nil
}

func (a *App) download(ctx context.Context, client *Client, format string, args []string) error {
	fs := flag.NewFlagSet("download", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	destination := fs.String("output", "", "destination ZIP path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("download requires one job ID")
	}
	if *destination == "" {
		*destination = "job-" + fs.Arg(0) + ".zip"
	}
	resp, err := client.Do(ctx, http.MethodGet, "/api/v1/jobs/"+fs.Arg(0)+"/archive.zip", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dir := filepath.Dir(*destination)
	tmp, err := os.CreateTemp(dir, ".jobdock-download-*.zip")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(tmpName)
		}
	}()
	if _, err = io.Copy(tmp, resp.Body); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpName, *destination); err != nil {
		return err
	}
	ok = true
	if format == "json" {
		return writeJSON(a.Out, map[string]string{"path": *destination})
	}
	fmt.Fprintln(a.Out, *destination)
	return nil
}

func parsePairs(values []string) (map[string]string, error) {
	result := map[string]string{}
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("expected KEY=VALUE, got %q", value)
		}
		result[parts[0]] = parts[1]
	}
	return result, nil
}
func (a *App) env(key, fallback string) string {
	if value := a.getenv(key); value != "" {
		return value
	}
	return fallback
}
func (a *App) fail(format string, exit int, code, message string) int {
	if format == "json" {
		_ = writeJSON(a.Err, map[string]any{"error": map[string]any{"code": code, "message": message}, "exit_code": exit})
	} else {
		fmt.Fprintf(a.Err, "Error: %s (%s)\n", message, code)
	}
	return exit
}
func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
