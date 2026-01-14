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

	"github.com/ai-shift/opencode/pkg/opencode"
)

//go:embed example_config
var configFS embed.FS

func main() {
	query := flag.String("query", "", "Message to send to opencode")
	flag.Parse()

	if *query == "" {
		fmt.Println("Usage: example -query \"your message\"")
		flag.Usage()
		os.Exit(1)
	}

	tmpDir, err := os.MkdirTemp("", "opencode-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Copy embedded config files to temp directory
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
		destPath := filepath.Join(tmpDir, relPath)

		return os.WriteFile(destPath, data, 0644)
	}); err != nil {
		log.Fatalf("Failed to copy config files: %v", err)
	}

	cfg := opencode.Config{
		ConfigDir: tmpDir,
		APIKey:    os.Getenv("OPENCODE_API_KEY"),
	}

	oc := opencode.New(cfg)

	fmt.Printf("Starting opencode with config: %s\n", tmpDir)

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
			if event.Type == "message.updated" {
				if info, ok := event.Properties["info"].(map[string]interface{}); ok {
					if role, ok := info["role"].(string); ok && role == "assistant" {
						if msgID, ok := info["id"].(string); ok {
							assistantMessageID = msgID
						}
					}
					if finish, ok := info["finish"].(string); ok && finish == "stop" {
						if receivedText {
							fmt.Println()
							os.Exit(0)
						}
					}
				}
			} else if event.Type == "message.part.updated" {
				if part, ok := event.Properties["part"].(map[string]interface{}); ok {
					if msgID, ok := part["messageID"].(string); ok && msgID == assistantMessageID {
						if partType, ok := part["type"].(string); ok && partType == "text" {
							if text, ok := part["text"].(string); ok && text != "" {
								if !receivedText {
									fmt.Print("Assistant: ")
									receivedText = true
								}
								fmt.Print(text)
							}
						}
					}
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
