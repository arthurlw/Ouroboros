package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type FailureKind int

const (
	FailureNone FailureKind = iota
	FailureBuild
	FailureTest
	FailureTimeout
	FailureUnknown
)

func (f FailureKind) String() string {
	switch f {
	case FailureNone:
		return "None"
	case FailureBuild:
		return "BuildFailure"
	case FailureTest:
		return "TestFailure"
	case FailureTimeout:
		return "Timeout"
	default:
		return "Unknown"
	}
}

type Result struct {
	ExitCode    int64
	Stdout      string
	Stderr      string
	Passed      bool
	FailureKind FailureKind
}

type Client struct {
	cli        *client.Client
	image      string
	stagingDir string
	cacheVol   string
}

func NewSandbox(image string, stagingDir string) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create staging dir: %w", err)
	}

	// NEW: Ensure we have a cache volume
	cacheName := "ouroboros-gocache"

	return &Client{
		cli:        cli,
		image:      image,
		stagingDir: stagingDir,
		cacheVol:   cacheName,
	}, nil
}

// ExecuteTest runs "goimports" then "go test"
func (s *Client) ExecuteTest(ctx context.Context, code string, test string) (*Result, error) {
	if err := s.writeToStaging("main.go", code); err != nil { return nil, err }
	if err := s.writeToStaging("main_test.go", test); err != nil { return nil, err }
	if err := s.writeToStaging("go.mod", fmt.Sprintf("module generated\n\ngo %s\n", GoVersion)); err != nil { return nil, err }

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	// Run goimports first to fix imports, then run tests
	return s.runContainer(ctx, []string{"sh", "-c", "goimports -w main.go main_test.go && go test -v ./..."})
}

// RunTool writes the code and runs "go run main.go [args]"
func (s *Client) RunTool(ctx context.Context, code string, args []string) (*Result, error) {
	// 1. Write the latest code
	if err := s.writeToStaging("main.go", code); err != nil { return nil, err }
	if err := s.writeToStaging("go.mod", fmt.Sprintf("module generated\n\ngo %s\n", GoVersion)); err != nil { return nil, err }

	// 2. Run with timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Construct command: go run main.go arg1 arg2 ...
	cmd := append([]string{"go", "run", "main.go"}, args...)

	return s.runContainer(ctx, cmd)
}

// ExecuteTool is the interface method; we force usage of RunTool now
func (s *Client) ExecuteTool(ctx context.Context, toolName string, args []string) (*Result, error) {
	return nil, fmt.Errorf("use RunTool with code content instead")
}

func (s *Client) writeToStaging(filename, content string) error {
	return os.WriteFile(filepath.Join(s.stagingDir, filename), []byte(content), 0644)
}

func (s *Client) runContainer(ctx context.Context, cmd []string) (*Result, error) {
	config := &container.Config{
		Image:        s.image,
		Cmd:          cmd,
		WorkingDir:   "/workspace",
		AttachStdout: true,
		AttachStderr: true,
		Env:          []string{"GOCACHE=/go-cache", "GOMODCACHE=/go-mod-cache"},
	}

	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{Type: mount.TypeBind, Source: s.stagingDir, Target: "/workspace"},
			{Type: mount.TypeVolume, Source: s.cacheVol, Target: "/go-cache"},
			{Type: mount.TypeVolume, Source: s.cacheVol + "-mod", Target: "/go-mod-cache"},
		},
		Resources:   container.Resources{Memory: 1024 * 1024 * 1024, NanoCPUs: 2 * 1e9},
		NetworkMode: "none",
		AutoRemove:  true,
	}

	resp, err := s.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil { return nil, fmt.Errorf("create error: %w", err) }

	if err := s.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("start error: %w", err)
	}

	out, _ := s.cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true})
	defer out.Close()

	stdoutBuf, stderrBuf := new(bytes.Buffer), new(bytes.Buffer)
	stdcopy.StdCopy(stdoutBuf, stderrBuf, out)

	statusCh, errCh := s.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return nil, err
	case status := <-statusCh:
		result := &Result{
			ExitCode: status.StatusCode,
			Stdout:   stdoutBuf.String(),
			Stderr:   stderrBuf.String(),
			Passed:   status.StatusCode == 0,
		}
		if status.StatusCode == 0 {
			result.FailureKind = FailureNone
		}
		return result, nil
	case <-ctx.Done():
		s.cli.ContainerKill(context.Background(), resp.ID, "SIGKILL")
		// Return a Result with timeout information
		return &Result{
			ExitCode:    -1,
			Stdout:      stdoutBuf.String(),
			Stderr:      "timeout",
			Passed:      false,
			FailureKind: FailureTimeout,
		}, fmt.Errorf("container execution timed out")
	}
}
