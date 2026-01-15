package opencode

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	ConfigDir string
	Addr      string
	APIKey    string
}

type OpenCode struct {
	config Config
	cmd    *exec.Cmd
	client *http.Client
	mu     sync.Mutex
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

	args := []string{"serve"}

	hostname := "127.0.0.1"
	args = append(args, "--hostname", hostname, "--port", fmt.Sprintf("%d", port))

	cmd := exec.Command("opencode", args...)

	// Set environment variables
	cmd.Env = os.Environ()

	configDir := oc.config.ConfigDir
	if configDir == "" {
		configDir = os.Getenv("OPENCODE_CONFIG_DIR")
	}

	if configDir != "" {
		// Set HOME and XDG_CONFIG_HOME to isolate config completely
		cmd.Env = append(cmd.Env, fmt.Sprintf("HOME=%s", configDir))
		cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_CONFIG_HOME=%s", configDir))
		cmd.Env = append(cmd.Env, fmt.Sprintf("OPENCODE_CONFIG_DIR=%s", configDir))
		slog.Info("Using isolated config directory", "dir", configDir)
	} else {
		slog.Info("Using system config directory")
	}

	if oc.config.APIKey != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("OPENCODE_API_KEY=%s", oc.config.APIKey))
		slog.Info("Set OPENCODE_API_KEY environment variable")
	}

	oc.cmd = cmd

	// Redirect stderr to see error messages
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	slog.Info("Starting opencode", "args", cmd.Args)

	if err := oc.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start opencode: %w", err)
	}
	slog.Info("OpenCode process started", "pid", oc.cmd.Process.Pid)

	go func() {
		state, err := oc.cmd.Process.Wait()
		if err != nil {
			slog.Error("OpenCode process exited with error", "pid", oc.cmd.Process.Pid, "err", err, "state", state)
		} else {
			slog.Info("OpenCode process exited", "pid", oc.cmd.Process.Pid, "state", state)
		}
	}()

	time.Sleep(500 * time.Millisecond)

	process, err := os.FindProcess(oc.cmd.Process.Pid)
	if err != nil {
		return fmt.Errorf("opencode process exited immediately")
	}

	if err := process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("opencode process failed to start")
	}

	slog.Info("OpenCode process confirmed running", "pid", oc.cmd.Process.Pid)
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

func (oc *OpenCode) WaitForReady(maxAttempts int) error {
	slog.Info("Waiting for OpenCode to be ready", "addr", oc.config.Addr, "maxAttempts", maxAttempts)
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < maxAttempts; i++ {
		resp, err := client.Get(fmt.Sprintf("http://%s/global/health", oc.config.Addr))
		if err == nil {
			resp.Body.Close()
			slog.Info("OpenCode is ready", "addr", oc.config.Addr, "attempt", i+1)
			return nil
		}
		if i%10 == 0 {
			slog.Debug("Waiting for OpenCode...", "attempt", i+1, "err", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("OpenCode not ready after %d attempts", maxAttempts)
}
