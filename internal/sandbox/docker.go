package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// Result captures the outcome of a critique session.
type Result struct {
	ExitCode int64
	Stdout   string
	Stderr   string
	Passed   bool
}

// Client wraps the Docker API.
type Client struct {
	cli        *client.Client
	image      string
	stagingDir string
}

// NewSandbox initializes the connection to Docker.
func NewSandbox(image string, stagingDir string) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	// Ensure staging directory exists
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create staging dir: %w", err)
	}

	return &Client{
		cli:        cli,
		image:      image,
		stagingDir: stagingDir,
	}, nil
}

// ExecuteTest writes the code and test to disk, then runs "go test".
func (s *Client) ExecuteTest(ctx context.Context, code string, test string) (*Result, error) {
	// 1. Write files to the staging directory
	if err := s.writeToStaging("main.go", code); err != nil {
		return nil, err
	}
	if err := s.writeToStaging("main_test.go", test); err != nil {
		return nil, err
	}

	// 2. Execute with strict timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return s.runContainer(ctx, []string{"go", "test", "-v", "./..."})
}

// ExecuteBenchmark runs "go test -bench" for Optimization Mode.
func (s *Client) ExecuteBenchmark(ctx context.Context, code string) (*Result, error) {
	// 1. Write file (Benchmarks usually live in the test file)
	if err := s.writeToStaging("main_test.go", code); err != nil {
		return nil, err
	}

	// Higher timeout for benchmarks
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	return s.runContainer(ctx, []string{"go", "test", "-bench=.", "-benchmem", "./..."})
}

// ExecuteTool runs a specific tool command (used for "Existing Tool" logic).
func (s *Client) ExecuteTool(ctx context.Context, toolName string, args []string) (*Result, error) {
	// For V1, we assume the tool is built/available in the path.
	// In a real scenario, you might mount the 'verified' folder.
	// This is a placeholder for the logic to run verified binary.
	return &Result{Stdout: "Tool execution not yet fully implemented in V1 Docker bridge", Passed: true}, nil
}

// --- Internal Helpers ---

func (s *Client) writeToStaging(filename, content string) error {
	path := filepath.Join(s.stagingDir, filename)
	return os.WriteFile(path, []byte(content), 0644)
}

func (s *Client) runContainer(ctx context.Context, cmd []string) (*Result, error) {
	// 1. Configure Container
	config := &container.Config{
		Image:        s.image,
		Cmd:          cmd,
		WorkingDir:   "/workspace",
		AttachStdout: true,
		AttachStderr: true,
	}

	// 2. Configure Host (Mount Staging Dir)
	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:/workspace", s.stagingDir),
		},
		Resources: container.Resources{
			Memory:   512 * 1024 * 1024, // 512MB limit
			NanoCPUs: 1 * 1e9,           // 1 CPU core
		},
		NetworkMode: "none", // Security: No internet
		AutoRemove:  true,   // Cleanup
	}

	// 3. Create
	resp, err := s.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// 4. Start
	if err := s.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// 5. Capture Logs
	out, err := s.cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}
	defer out.Close()

	stdoutBuf, stderrBuf := new(bytes.Buffer), new(bytes.Buffer)
	_, err = stdcopy.StdCopy(stdoutBuf, stderrBuf, out)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read logs: %w", err)
	}

	// 6. Wait for Exit
	statusCh, errCh := s.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("error waiting for container: %w", err)
		}
	case status := <-statusCh:
		return &Result{
			ExitCode: status.StatusCode,
			Stdout:   stdoutBuf.String(),
			Stderr:   stderrBuf.String(),
			Passed:   status.StatusCode == 0,
		}, nil
	case <-ctx.Done():
		_ = s.cli.ContainerKill(context.Background(), resp.ID, "SIGKILL")
		return nil, ctx.Err()
	}
	return nil, fmt.Errorf("unexpected execution flow")
}
