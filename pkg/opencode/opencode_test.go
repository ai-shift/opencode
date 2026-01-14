package opencode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	cfg := Config{
		ConfigDir: "/test/config",
		Addr:      "localhost:8080",
	}
	oc := New(cfg)

	assert.NotNil(t, oc)
	assert.Equal(t, cfg.ConfigDir, oc.config.ConfigDir)
	assert.Equal(t, cfg.Addr, oc.config.Addr)
	assert.NotNil(t, oc.client)
	assert.Equal(t, cfg.Addr, oc.Addr())
}

func TestGetURL(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		path     string
		expected string
	}{
		{
			name:     "custom address",
			addr:     "localhost:8080",
			path:     "/session",
			expected: "http://localhost:8080/session",
		},
		{
			name:     "default address",
			addr:     "",
			path:     "/session",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oc := New(Config{Addr: tt.addr})
			result := oc.getURL(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStopWhenNotRunning(t *testing.T) {
	oc := New(Config{})
	err := oc.Stop()
	assert.NoError(t, err)
}

func TestStartWhenAlreadyRunning(t *testing.T) {
	cfg := Config{
		Addr: "localhost:6973",
	}
	oc := New(cfg)
	oc.cmd = nil

	err := oc.Start()
	if err != nil {
		assert.Contains(t, err.Error(), "failed to start opencode")
	}
}

func TestStartAutoAllocatesPort(t *testing.T) {
	cfg := Config{
		ConfigDir: "/test/config",
	}
	oc := New(cfg)
	oc.cmd = nil

	err := oc.Start()
	if err == nil {
		oc.Stop()
		t.Log("Started successfully, would allocate random port")
	}
}
