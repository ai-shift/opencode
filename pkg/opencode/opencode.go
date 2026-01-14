package opencode

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
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

type Session struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	ProjectID string `json:"projectID"`
	Directory string `json:"directory"`
	ParentID  string `json:"parentID,omitempty"`
	Title     string `json:"title"`
	Version   string `json:"version"`
}

type MessagePart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Message struct {
	ID        string        `json:"id"`
	SessionID string        `json:"sessionID"`
	Role      string        `json:"role"`
	Parts     []MessagePart `json:"parts"`
}

type Event struct {
	Type       string
	Properties map[string]interface{}
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

func (oc *OpenCode) getURL(path string) string {
	addr := oc.config.Addr
	if addr == "" {
		return ""
	}
	return fmt.Sprintf("http://%s%s", addr, path)
}

func (oc *OpenCode) ListSessions() ([]Session, error) {
	slog.Info("Listing sessions")
	req, err := http.NewRequest("GET", oc.getURL("/session"), nil)
	if err != nil {
		return nil, err
	}

	configDir := oc.config.ConfigDir
	if configDir == "" {
		configDir = os.Getenv("OPENCODE_CONFIG_DIR")
	}

	if configDir != "" {
		q := req.URL.Query()
		q.Add("directory", configDir)
		req.URL.RawQuery = q.Encode()
	}

	resp, err := oc.client.Do(req)
	if err != nil {
		slog.Error("Failed to list sessions", "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Unexpected status code listing sessions", "status", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var sessions []Session
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		slog.Error("Failed to decode sessions", "err", err)
		return nil, err
	}

	slog.Info("Sessions retrieved", "count", len(sessions))
	return sessions, nil
}

func (oc *OpenCode) CreateSession(title string) (*Session, error) {
	slog.Info("Creating session", "title", title)
	body := map[string]string{
		"title": title,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", oc.getURL("/session"), bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	configDir := oc.config.ConfigDir
	if configDir == "" {
		configDir = os.Getenv("OPENCODE_CONFIG_DIR")
	}

	if configDir != "" {
		q := req.URL.Query()
		q.Add("directory", configDir)
		req.URL.RawQuery = q.Encode()
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := oc.client.Do(req)
	if err != nil {
		slog.Error("Failed to create session", "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Unexpected status code creating session", "status", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		slog.Error("Failed to decode session", "err", err)
		return nil, err
	}

	slog.Info("Session created", "id", session.ID, "title", session.Title)
	return &session, nil
}

func (oc *OpenCode) SendMessage(sessionID, text string) (*Message, error) {
	slog.Info("Sending message", "sessionID", sessionID, "text", text)
	body := map[string]interface{}{
		"parts": []map[string]interface{}{
			{
				"type": "text",
				"text": text,
			},
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", oc.getURL(fmt.Sprintf("/session/%s/message", sessionID)), bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	configDir := oc.config.ConfigDir
	if configDir == "" {
		configDir = os.Getenv("OPENCODE_CONFIG_DIR")
	}

	if configDir != "" {
		q := req.URL.Query()
		q.Add("directory", configDir)
		req.URL.RawQuery = q.Encode()
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := oc.client.Do(req)
	if err != nil {
		slog.Error("Failed to send message", "sessionID", sessionID, "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Unexpected status code sending message", "status", resp.StatusCode, "sessionID", sessionID)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var response struct {
		Info  Message `json:"info"`
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"parts"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		slog.Error("Failed to decode message response", "err", err)
		return nil, err
	}

	slog.Info("Message sent", "messageID", response.Info.ID, "sessionID", sessionID)
	return &response.Info, nil
}

func (oc *OpenCode) StreamEvents(callback func(Event)) error {
	slog.Info("Starting event stream")
	req, err := http.NewRequest("GET", oc.getURL("/event"), nil)
	if err != nil {
		slog.Error("Failed to create event stream request", "err", err)
		return err
	}

	configDir := oc.config.ConfigDir
	if configDir == "" {
		configDir = os.Getenv("OPENCODE_CONFIG_DIR")
	}

	if configDir != "" {
		q := req.URL.Query()
		q.Add("directory", configDir)
		req.URL.RawQuery = q.Encode()
	}

	resp, err := oc.client.Do(req)
	if err != nil {
		slog.Error("Failed to start event stream", "err", err)
		return err
	}
	defer resp.Body.Close()
	slog.Info("Event stream connected")

	// Parse Server-Sent Events (SSE) format
	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer size to handle large events (e.g. file listings)
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	for scanner.Scan() {
		line := scanner.Text()

		// SSE lines starting with "data: " contain the event payload
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			var event struct {
				Type       string                 `json:"type"`
				Properties map[string]interface{} `json:"properties,omitempty"`
			}

			if err := json.Unmarshal([]byte(data), &event); err != nil {
				slog.Error("Error decoding event", "err", err, "data", data)
				continue
			}

			slog.Debug("Received event", "type", event.Type)
			callback(Event{
				Type:       event.Type,
				Properties: event.Properties,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Error("Event stream error", "err", err)
		return err
	}

	slog.Info("Event stream ended")
	return nil
}
