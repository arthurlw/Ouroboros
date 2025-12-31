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
	// 1. Write the Code
	if err := s.writeToStaging("main.go", code); err != nil {
		return nil, err
	}
	// 2. Write the Test
	if err := s.writeToStaging("main_test.go", test); err != nil {
		return nil, err
	}

	// 3. Write go.mod (Renamed module to 'generated' for safety)
	goModContent := "module generated\n\ngo 1.23\n"
	if err := s.writeToStaging("go.mod", goModContent); err != nil {
		return nil, err
	}

	// 4. Execute with timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return s.runContainer(ctx, []string{"go", "test", "-v", "./..."})
}

// ExecuteBenchmark runs "go test -bench".
func (s *Client) ExecuteBenchmark(ctx context.Context, code string) (*Result, error) {
	if err := s.writeToStaging("main_test.go", code); err != nil {
		return nil, err
	}
	// Also ensure go.mod exists for benchmarks
	_ = s.writeToStaging("go.mod", "module tool\n\ngo 1.23\n")

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	return s.runContainer(ctx, []string{"go", "test", "-bench=.", "-benchmem", "./..."})
}

// ExecuteTool runs a specific tool command.
func (s *Client) ExecuteTool(ctx context.Context, toolName string, args []string) (*Result, error) {
	// Placeholder for V1
	return &Result{Stdout: "Tool execution simulated.", Passed: true}, nil
}

// --- Internal Helpers ---

func (s *Client) writeToStaging(filename, content string) error {
	path := filepath.Join(s.stagingDir, filename)
	return os.WriteFile(path, []byte(content), 0644)
}

func (s *Client) runContainer(ctx context.Context, cmd []string) (*Result, error) {
	// 1. Configure
	config := &container.Config{
		Image:        s.image,
		Cmd:          cmd,
		WorkingDir:   "/workspace",
		AttachStdout: true,
		AttachStderr: true,
	}

	// 2. Mount Host Directory
	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:/workspace", s.stagingDir),
		},
		Resources: container.Resources{
			Memory:   512 * 1024 * 1024,
			NanoCPUs: 1 * 1e9,
		},
		NetworkMode: "none",
		AutoRemove:  true,
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

	// 5. Logs
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

	// 6. Wait
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
