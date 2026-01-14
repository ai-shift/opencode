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
	query := flag.String("query", "", "Message to send to opencode")
	dir := flag.String("dir", "", "Directory for opencode to operate in (defaults to current directory)")
	flag.Parse()

	if *query == "" {
		fmt.Println("Usage: example -query \"your message\" [-dir /path/to/session/dir]")
		flag.Usage()
		os.Exit(1)
	}

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

	fmt.Printf("Starting opencode in directory: %s\n", sessionDir)

	if err := oc.Start(); err != nil {
		log.Fatalf("Failed to start opencode: %v", err)
	}
	defer oc.Stop()

	if err := oc.WaitForReady(240); err != nil {
		log.Fatalf("Failed to connect to opencode: %v", err)
	}
	fmt.Println("Connected!")

	sessions, err := oc.ListSessions()
	if err != nil {
		log.Fatalf("Failed to list sessions: %v", err)
	}

	var sessionID string
	if len(sessions) > 0 {
		sessionID = sessions[0].ID
		fmt.Printf("Using existing session: %s (%s)\n", sessions[0].ID, sessions[0].Title)
	} else {
		session, err := oc.CreateSession("Example Session")
		if err != nil {
			log.Fatalf("Failed to create session: %v", err)
		}
		sessionID = session.ID
		fmt.Printf("Created new session: %s\n", sessionID)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	eventChan := make(chan opencode.Event, 1000)
	errorChan := make(chan error, 1)

	go func() {
		errorChan <- oc.StreamEvents(func(event opencode.Event) {
			eventChan <- event
		})
	}()

	fmt.Printf("Sending message: %s\n", *query)
	if _, err := oc.SendMessage(sessionID, *query); err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}

	receivedText := false
	var assistantMessageID string

	for {
		select {
		case event := <-eventChan:
			switch evt := event.(type) {
			case *opencode.MessageUpdatedEvent:
				if evt.Properties.Info.Role == "assistant" {
					assistantMessageID = evt.Properties.Info.ID

					// Check if message is finished
					if evt.Properties.Info.Finish != nil && *evt.Properties.Info.Finish == "stop" {
						if receivedText {
							fmt.Println()
							os.Exit(0)
						}
					}
				}

			case *opencode.MessagePartUpdatedEvent:
				part := evt.Properties.Part
				if part.MessageID == assistantMessageID && part.Type == "text" && part.Text != "" {
					if !receivedText {
						fmt.Print("Assistant: ")
						receivedText = true
					}
					fmt.Print(part.Text)
				}
			}
		case err := <-errorChan:
			if err != nil {
				log.Printf("Stream error: %v", err)
			}
			return
		case <-sigChan:
			fmt.Println("\nInterrupted by user")
			return
		}
	}
}
