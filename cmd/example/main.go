package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ai-shift/opencode"
)

//go:embed example_config
var configFS embed.FS

func main() {
	dir := flag.String("dir", "", "Directory for opencode to operate in (defaults to current directory)")
	flag.Parse()

	// Get current working directory as default
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// Determine session directory (defaults to current directory)
	sessionDir := *dir
	if sessionDir == "" {
		sessionDir = cwd
	}

	// Ensure session directory exists
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		log.Fatalf("Failed to create directory %s: %v", sessionDir, err)
	}

	// Copy embedded config files to directory (always update from embedded FS)
	if err := fs.WalkDir(configFS, "example_config", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := configFS.ReadFile(path)
		if err != nil {
			return err
		}

		// Get the relative path without the "example_config/" prefix
		relPath := filepath.Base(path)
		destPath := filepath.Join(sessionDir, relPath)

		return os.WriteFile(destPath, data, 0644)
	}); err != nil {
		log.Fatalf("Failed to copy config files: %v", err)
	}

	cfg := opencode.Config{
		ConfigDir: sessionDir,
		APIKey:    os.Getenv("OPENCODE_API_KEY"),
	}

	oc := opencode.New(cfg)

	fmt.Printf("Starting OpenCode server in directory: %s\n", sessionDir)

	if err := oc.Start(); err != nil {
		log.Fatalf("Failed to start opencode: %v", err)
	}
	defer oc.Stop()

	if err := oc.WaitForReady(240); err != nil {
		log.Fatalf("Failed to connect to opencode: %v", err)
	}

	fmt.Printf("OpenCode server is ready at: http://%s\n", oc.Addr())
	fmt.Println("Press Ctrl+C to stop the server")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nStopping OpenCode server...")
}
