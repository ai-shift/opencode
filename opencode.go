package opencode

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Config struct {
	Addr     string
	ConfigFS fs.FS
	CWD      string
}

type OpenCode struct {
	config    Config
	cmd       *exec.Cmd
	client    *http.Client
	configDir string
	mu        sync.Mutex
}

func New(cfg Config) *OpenCode {
	return &OpenCode{
		config: cfg,
		client: &http.Client{},
	}
}

func (oc *OpenCode) Start() error {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	if oc.cmd != nil && oc.cmd.Process != nil {
		return fmt.Errorf("opencode is already running")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to get free port: %w", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	listener.Close()
	port := addr.Port
	oc.config.Addr = fmt.Sprintf("127.0.0.1:%d", port)
	slog.Info("Allocated random port", "port", port)

	if oc.config.ConfigFS != nil {
		hashBytes := make([]byte, 8)
		if _, err := rand.Read(hashBytes); err != nil {
			return fmt.Errorf("failed to generate random hash: %w", err)
		}
		hash := hex.EncodeToString(hashBytes)
		oc.configDir = filepath.Join("/tmp", fmt.Sprintf("opencode_%s", hash))

		if err := os.MkdirAll(oc.configDir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
		slog.Info("Created config directory", "path", oc.configDir)

		if err := fs.WalkDir(oc.config.ConfigFS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			content, err := fs.ReadFile(oc.config.ConfigFS, path)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path, err)
			}

			// Expand environment variables in the content
			expandedContent := []byte(os.ExpandEnv(string(content)))

			destPath := filepath.Join(oc.configDir, path)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("failed to create directory for %s: %w", destPath, err)
			}

			if err := os.WriteFile(destPath, expandedContent, 0644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", destPath, err)
			}

			return nil
		}); err != nil {
			return fmt.Errorf("failed to walk config fs: %w", err)
		}
	}

	args := []string{"serve"}

	hostname := "127.0.0.1"
	args = append(args, "--hostname", hostname, "--port", fmt.Sprintf("%d", port))

	oc.cmd = exec.Command("opencode", args...)
	oc.cmd.Env = os.Environ()

	if oc.configDir != "" {
		configJSONPath := filepath.Join(oc.configDir, "config.json")
		oc.cmd.Env = append(oc.cmd.Env,
			fmt.Sprintf("OPENCODE_CONFIG=%s", configJSONPath),
			fmt.Sprintf("OPENCODE_CONFIG_DIR=%s", oc.configDir),
		)
		slog.Info("Set config environment variables", "config", configJSONPath, "dir", oc.configDir)
	}

	if oc.config.CWD != "" {
		oc.cmd.Dir = oc.config.CWD
		slog.Info("Set working directory for opencode process", "cwd", oc.config.CWD)
	}

	// Redirect stderr to see error messages
	oc.cmd.Stderr = os.Stderr
	oc.cmd.Stdout = os.Stdout

	slog.Info("Starting opencode", "args", oc.cmd.Args)

	if err := oc.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start opencode: %w", err)
	}
	slog.Info("OpenCode process started", "pid", oc.cmd.Process.Pid)

	return nil
}

func (oc *OpenCode) Stop() error {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	if oc.cmd == nil || oc.cmd.Process == nil {
		slog.Info("OpenCode not running, nothing to stop")
		return nil
	}

	pid := oc.cmd.Process.Pid
	slog.Info("Stopping OpenCode", "pid", pid)
	if err := oc.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to stop opencode: %w", err)
	}

	oc.cmd = nil
	slog.Info("OpenCode stopped", "pid", pid)
	return nil
}

func (oc *OpenCode) Addr() string {
	return oc.config.Addr
}

func (oc *OpenCode) WaitForReady(ctx context.Context, maybeTimeout ...time.Duration) error {
	timeout := 15 * time.Second
	if len(maybeTimeout) > 0 {
		timeout = maybeTimeout[0]
	}
	var cancel context.CancelFunc
	if len(maybeTimeout) > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		cancel = func() {}
	}
	defer cancel()
	slog.Info("Waiting for OpenCode to be ready", "addr", oc.config.Addr, "timeout", timeout)
	readyChan := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s/global/health", oc.config.Addr), nil)
				resp, err := http.DefaultClient.Do(req)
				if err == nil {
					resp.Body.Close()
					slog.Info("OpenCode is ready", "addr", oc.config.Addr, "attempt", i+1)
					readyChan <- struct{}{}
					return
				}
				if i%10 == 0 {
					slog.Debug("Waiting for OpenCode...", "attempt", i+1, "err", err)
				}
			}
		}
	}()
	select {
	case <-readyChan:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("opencode is not ready after %s", timeout)
	}
}

func (oc *OpenCode) Cleanup() error {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	if oc.configDir == "" {
		return nil
	}

	slog.Info("Cleaning up config directory", "path", oc.configDir)
	if err := os.RemoveAll(oc.configDir); err != nil {
		return fmt.Errorf("failed to remove config directory: %w", err)
	}

	oc.configDir = ""
	slog.Info("Config directory removed")
	return nil
}
