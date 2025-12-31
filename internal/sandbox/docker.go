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

type Result struct {
	ExitCode int64
	Stdout   string
	Stderr   string
	Passed   bool
}

type Client struct {
	cli        *client.Client
	image      string
	stagingDir string
	cacheVol   string // NEW: Name of the persistent Docker volume for Go Cache
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
	// (In production you'd check if it exists, here Docker creates it automatically on mount)

	return &Client{
		cli:        cli,
		image:      image,
		stagingDir: stagingDir,
		cacheVol:   cacheName,
	}, nil
}

func (s *Client) ExecuteTest(ctx context.Context, code string, test string) (*Result, error) {
	// 1. Write Code (Same as before)
	if err := s.writeToStaging("main.go", code); err != nil { return nil, err }
	if err := s.writeToStaging("main_test.go", test); err != nil { return nil, err }

	// 2. Write go.mod (Crucial for module support)
	if err := s.writeToStaging("go.mod", "module generated\n\ngo 1.23\n"); err != nil { return nil, err }

	// 3. Run with Caching
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second) // Slightly higher timeout for first run
	defer cancel()

	return s.runContainer(ctx, []string{"go", "test", "-v", "./..."})
}

func (s *Client) ExecuteTool(ctx context.Context, toolName string, args []string) (*Result, error) {
	// Stub for V1
	return &Result{Stdout: "Tool execution simulated", Passed: true}, nil
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
		// NEW: Environment variables to force Go to use the mounted cache
		Env: []string{"GOCACHE=/go-cache", "GOMODCACHE=/go-mod-cache"},
	}

	hostConfig := &container.HostConfig{
		// NEW: Mounts strategy
		Mounts: []mount.Mount{
			// 1. The Code (Bind Mount)
			{
				Type:   mount.TypeBind,
				Source: s.stagingDir,
				Target: "/workspace",
			},
			// 2. The Build Cache (Volume - Fast!)
			{
				Type:   mount.TypeVolume,
				Source: s.cacheVol,
				Target: "/go-cache",
			},
			// 3. The Mod Cache (Volume - Fast!)
			{
				Type:   mount.TypeVolume,
				Source: s.cacheVol + "-mod",
				Target: "/go-mod-cache",
			},
		},
		Resources: container.Resources{
			Memory:   1024 * 1024 * 1024, // Bumped to 1GB for faster builds
			NanoCPUs: 2 * 1e9,           // Bumped to 2 CPUs
		},
		NetworkMode: "none",
		AutoRemove:  true,
	}

	resp, err := s.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("create error: %w", err)
	}

	if err := s.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("start error: %w", err)
	}

	// ... Log capture logic (Same as previous) ...
	out, _ := s.cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true})
	defer out.Close()
	stdoutBuf, stderrBuf := new(bytes.Buffer), new(bytes.Buffer)
	stdcopy.StdCopy(stdoutBuf, stderrBuf, out)

	statusCh, errCh := s.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return nil, err
	case status := <-statusCh:
		return &Result{
			ExitCode: status.StatusCode,
			Stdout:   stdoutBuf.String(),
			Stderr:   stderrBuf.String(),
			Passed:   status.StatusCode == 0,
		}, nil
	case <-ctx.Done():
		s.cli.ContainerKill(context.Background(), resp.ID, "SIGKILL")
		return nil, ctx.Err()
	}
}
