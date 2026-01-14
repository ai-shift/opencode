package opencode

import "encoding/json"

// Event represents a base event from OpenCode SSE stream
type Event interface {
	EventType() string
}

// BaseEvent contains common fields for all events
type BaseEvent struct {
	Type string `json:"type"`
}

func (e BaseEvent) EventType() string {
	return e.Type
}

// ServerConnectedEvent is emitted when the client connects to the server
type ServerConnectedEvent struct {
	BaseEvent
	Properties struct{} `json:"properties"`
}

// MessagePartUpdatedEvent is emitted when a message part is updated
type MessagePartUpdatedEvent struct {
	BaseEvent
	Properties struct {
		Part  Part   `json:"part"`
		Delta string `json:"delta,omitempty"`
	} `json:"properties"`
}

// MessageUpdatedEvent is emitted when a message is updated
type MessageUpdatedEvent struct {
	BaseEvent
	Properties struct {
		Info MessageInfo `json:"info"`
	} `json:"properties"`
}

// SessionUpdatedEvent is emitted when a session is updated
type SessionUpdatedEvent struct {
	BaseEvent
	Properties struct {
		Info Session `json:"info"`
	} `json:"properties"`
}

// SessionStatusEvent is emitted when a session status changes
type SessionStatusEvent struct {
	BaseEvent
	Properties struct {
		SessionID string        `json:"sessionID"`
		Status    SessionStatus `json:"status"`
	} `json:"properties"`
}

// SessionStatus represents the status of a session
type SessionStatus struct {
	Type string `json:"type"` // "idle", "busy", etc.
}

// MessageInfo represents message information (can be user or assistant)
type MessageInfo struct {
	ID        string  `json:"id"`
	SessionID string  `json:"sessionID"`
	Role      string  `json:"role"` // "user" or "assistant"
	ParentID  *string `json:"parentID,omitempty"`
	Time      struct {
		Created   int64  `json:"created"`
		Completed *int64 `json:"completed,omitempty"`
	} `json:"time"`

	// Assistant-specific fields
	ModelID    *string `json:"modelID,omitempty"`
	ProviderID *string `json:"providerID,omitempty"`
	Mode       *string `json:"mode,omitempty"`
	Agent      *string `json:"agent,omitempty"`
	Path       *struct {
		Cwd  string `json:"cwd"`
		Root string `json:"root"`
	} `json:"path,omitempty"`
	Cost   *float64 `json:"cost,omitempty"`
	Tokens *struct {
		Input     int `json:"input"`
		Output    int `json:"output"`
		Reasoning int `json:"reasoning"`
		Cache     struct {
			Read  int `json:"read"`
			Write int `json:"write"`
		} `json:"cache"`
	} `json:"tokens,omitempty"`
	Finish *string `json:"finish,omitempty"` // "stop", "length", "error", etc.
}

// Part represents a message part (can be text, tool, reasoning, etc.)
type Part struct {
	ID        string                 `json:"id"`
	SessionID string                 `json:"sessionID"`
	MessageID string                 `json:"messageID"`
	Type      string                 `json:"type"` // "text", "tool", "reasoning", "step-start", "step-finish", etc.
	Text      string                 `json:"text,omitempty"`
	Time      *PartTime              `json:"time,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`

	// Additional fields for different part types
	Synthetic *bool   `json:"synthetic,omitempty"`
	Ignored   *bool   `json:"ignored,omitempty"`
	Reason    *string `json:"reason,omitempty"` // For step-finish parts
	Cost      *float64 `json:"cost,omitempty"`
	Tokens    *struct {
		Input     int `json:"input"`
		Output    int `json:"output"`
		Reasoning int `json:"reasoning"`
		Cache     struct {
			Read  int `json:"read"`
			Write int `json:"write"`
		} `json:"cache"`
	} `json:"tokens,omitempty"`
}

// PartTime represents timing information for a part
type PartTime struct {
	Start int64  `json:"start"`
	End   *int64 `json:"end,omitempty"`
}

// UnknownEvent represents an event type we don't explicitly handle
type UnknownEvent struct {
	BaseEvent
	Properties map[string]interface{} `json:"properties"`
}

// ParseEvent parses a raw JSON event into a concrete event type
func ParseEvent(data []byte) (Event, error) {
	var base BaseEvent
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, err
	}

	switch base.Type {
	case "server.connected":
		var evt ServerConnectedEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		return &evt, nil

	case "message.part.updated":
		var evt MessagePartUpdatedEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		return &evt, nil

	case "message.updated":
		var evt MessageUpdatedEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		return &evt, nil

	case "session.updated":
		var evt SessionUpdatedEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		return &evt, nil

	case "session.status":
		var evt SessionStatusEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		return &evt, nil

	default:
		// For unknown event types, parse into UnknownEvent
		var evt UnknownEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		return &evt, nil
	}
}
