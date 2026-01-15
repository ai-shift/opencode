package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ai-shift/opencode"
)

//go:embed .opencode
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

	// Create sub FS for config files
	subFS, err := fs.Sub(configFS, ".opencode")
	if err != nil {
		log.Fatalf("Failed to create sub FS: %v", err)
	}

	cfg := opencode.Config{
		CWD:      sessionDir,
		ConfigFS: subFS,
	}

	oc := opencode.New(cfg)

	fmt.Printf("Starting OpenCode server in directory: %s\n", sessionDir)

	go func() {
		if err := oc.Start(); err != nil {
			log.Fatalf("Failed to start opencode: %v", err)
		}
	}()
	defer oc.Stop()
	defer oc.Cleanup()

	if err := oc.WaitForReady(context.Background()); err != nil {
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
