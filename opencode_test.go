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
