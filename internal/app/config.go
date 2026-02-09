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
	Gameplay       GameplayConfig
	UI             UIConfig
}

type GameplayConfig struct {
	AutoCheckDefault    string
	AutoCheckDebounceMS int
}

type UIConfig struct {
	StyleVariant string
	MotionLevel  string
	MouseScope   string
}

func DefaultConfig() Config {
	return Config{
		SandboxMode: "auto",
		DevHTTP:     "127.0.0.1:17321",
		Gameplay: GameplayConfig{
			AutoCheckDefault:    "off",
			AutoCheckDebounceMS: 800,
		},
		UI: UIConfig{
			StyleVariant: "modern_arcade",
			MotionLevel:  "full",
			MouseScope:   "scoped",
		},
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
	switch c.Gameplay.AutoCheckDefault {
	case "", "off", "manual", "command_debounce", "command_and_fs_debounce":
	default:
		return fmt.Errorf("invalid gameplay auto-check mode %q", c.Gameplay.AutoCheckDefault)
	}
	if c.Gameplay.AutoCheckDefault == "" {
		c.Gameplay.AutoCheckDefault = "off"
	}
	if c.Gameplay.AutoCheckDebounceMS <= 0 {
		c.Gameplay.AutoCheckDebounceMS = 800
	}
	switch c.UI.StyleVariant {
	case "", "modern_arcade", "cozy_clean", "retro_terminal":
	default:
		return fmt.Errorf("invalid ui style variant %q", c.UI.StyleVariant)
	}
	if c.UI.StyleVariant == "" {
		c.UI.StyleVariant = "modern_arcade"
	}
	switch c.UI.MotionLevel {
	case "", "off", "reduced", "full":
	default:
		return fmt.Errorf("invalid ui motion level %q", c.UI.MotionLevel)
	}
	if c.UI.MotionLevel == "" {
		c.UI.MotionLevel = "full"
	}
	switch c.UI.MouseScope {
	case "", "off", "scoped", "full":
	default:
		return fmt.Errorf("invalid ui mouse scope %q", c.UI.MouseScope)
	}
	if c.UI.MouseScope == "" {
		c.UI.MouseScope = "scoped"
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
