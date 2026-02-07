package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config controls runtime behavior for the TUI app.
type Config struct {
	Dev            bool
	DevHTTP        string
	LogPath        string
	DebugLayout    bool
	SandboxMode    string
	DemoScenario   string
	EngineOverride string
	ASCIIOnly      bool
	DataDir        string
	KeepArtifacts  bool
}

func DefaultConfig() Config {
	return Config{
		SandboxMode: "auto",
		DevHTTP:     "127.0.0.1:17321",
	}
}

func (c *Config) Validate() error {
	switch c.SandboxMode {
	case "auto", "mock", "docker", "podman":
	default:
		return fmt.Errorf("invalid sandbox mode %q", c.SandboxMode)
	}

	if c.EngineOverride != "" && c.EngineOverride != "docker" && c.EngineOverride != "podman" {
		return fmt.Errorf("invalid engine override %q", c.EngineOverride)
	}

	if c.DataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("cannot resolve user home directory")
		}
		c.DataDir = filepath.Join(home, ".local", "share", "clidojo")
	}

	return nil
}
