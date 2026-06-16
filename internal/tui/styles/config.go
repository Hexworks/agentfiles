package styles

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
)

// ConfigFileName is the on-disk filename the theme loader looks for.
// JSON (not TOML) so the package stays in stdlib; the schema is flat
// — string hex or ANSI index values keyed by palette field name —
// so JSON's punctuation cost is negligible for the size of the file.
const ConfigFileName = "theme.json"

// configSchema mirrors [Palette] one-to-one but uses string values so
// hex (`#RRGGBB`) and ANSI indices (`"8"`, `"245"`) both round-trip
// through [lipgloss.Color]. Missing keys keep the [DefaultPalette]
// value for that role; empty string is treated as "unset", same as
// absent.
type configSchema struct {
	Muted   string `json:"muted,omitempty"`
	Cyan    string `json:"cyan,omitempty"`
	Red     string `json:"red,omitempty"`
	Green   string `json:"green,omitempty"`
	Yellow  string `json:"yellow,omitempty"`
	Magenta string `json:"magenta,omitempty"`

	MnemonicAccent string `json:"mnemonic_accent,omitempty"`
	MnemonicHL     string `json:"mnemonic_highlight,omitempty"`
	MnemonicText   string `json:"mnemonic_text,omitempty"`
}

// LoadConfig reads a theme override from path, merges it onto
// [DefaultPalette], and installs the result via [Apply]. A non-existent
// file is not an error — the function returns nil and the default
// palette stays installed. A present-but-malformed file returns a typed
// error so cmd/af can surface it and abort startup instead of
// silently falling back to defaults.
//
// When path is empty [DefaultConfigPath] is consulted; passing an
// explicit path is reserved for tests.
func LoadConfig(path string) error {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return &ConfigReadError{Path: path, Err: err}
	}
	var cfg configSchema
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &ConfigParseError{Path: path, Err: err}
	}
	Apply(mergePalette(DefaultPalette(), cfg))
	return nil
}

// DefaultConfigPath resolves $XDG_CONFIG_HOME/agentfiles/theme.json
// (falling back to ~/.config/agentfiles/theme.json on systems without
// the env var). Returns an error only when neither $HOME nor
// $XDG_CONFIG_HOME can be discovered.
func DefaultConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "agentfiles", ConfigFileName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "agentfiles", ConfigFileName), nil
}

// mergePalette returns a copy of base with every non-empty field on
// cfg overlaid. Used so a partial theme file (e.g. only `cyan`
// overridden) keeps the rest of the default palette intact.
func mergePalette(base Palette, cfg configSchema) Palette {
	overlay := func(dst *color.Color, raw string) {
		if raw == "" {
			return
		}
		*dst = lipgloss.Color(raw)
	}
	overlay(&base.Muted, cfg.Muted)
	overlay(&base.Cyan, cfg.Cyan)
	overlay(&base.Red, cfg.Red)
	overlay(&base.Green, cfg.Green)
	overlay(&base.Yellow, cfg.Yellow)
	overlay(&base.Magenta, cfg.Magenta)
	overlay(&base.MnemonicAccent, cfg.MnemonicAccent)
	overlay(&base.MnemonicHL, cfg.MnemonicHL)
	overlay(&base.MnemonicText, cfg.MnemonicText)
	return base
}

// ConfigReadError reports an I/O failure reading the theme file. The
// caller (cmd/af) renders the message and exits non-zero.
type ConfigReadError struct {
	Path string
	Err  error
}

func (e *ConfigReadError) Error() string {
	return fmt.Sprintf("read theme %s: %v", e.Path, e.Err)
}

func (e *ConfigReadError) Unwrap() error { return e.Err }

// ConfigParseError reports a JSON parse failure in the theme file.
type ConfigParseError struct {
	Path string
	Err  error
}

func (e *ConfigParseError) Error() string {
	return fmt.Sprintf("parse theme %s: %v", e.Path, e.Err)
}

func (e *ConfigParseError) Unwrap() error { return e.Err }
