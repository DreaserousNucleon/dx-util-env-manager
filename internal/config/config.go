package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const AppName = "de"

const DefaultProfile = "dev"

// DefaultConfigTemplate is the commented config.toml written on first `de config edit`.
const DefaultConfigTemplate = `# de configuration file
# See: de help | de config show

# Show values in plaintext by default (default: false)
# reveal = false

# Color output: "auto", "always", "never" (default: "auto")
# color = "auto"

# Active environment profile (default: "dev")
# Allowed: dev, staging, pre-prod, qe, prod
# profile = "dev"

# Override the store file path
# store_path = "/custom/path/store.json"

# Allow automatic store re-initialization when decryption fails after a key purge
# allow_reinit_on_decrypt_fail = false
`

var AllowedProfiles = []string{"dev", "staging", "pre-prod", "qe", "prod"}

// ValidateProfile returns an error if the profile name is not in the allowed set.
func ValidateProfile(name string) error {
	for _, p := range AllowedProfiles {
		if p == name {
			return nil
		}
	}
	return fmt.Errorf("invalid profile %q (allowed: %s)", name, strings.Join(AllowedProfiles, ", "))
}

type Config struct {
	Dir       string
	StorePath string
	// User preferences (from config.toml).
	Reveal                   bool   // default reveal mode
	Color                    string // "auto", "always", "never"
	Profile                  string // active environment profile
	AllowReinitOnDecryptFail bool   // purge unreadable store on decrypt failure when no key exists
}

func Load() (Config, error) {
	cfg, err := Default()
	if err != nil {
		return Config{}, err
	}

	if override := os.Getenv("DE_STORE_PATH"); override != "" {
		cfg.StorePath = override
		cfg.Dir = filepath.Dir(override)
	}

	if override := os.Getenv("DE_CONFIG_DIR"); override != "" {
		cfg.Dir = override
		cfg.StorePath = filepath.Join(override, "store.json")
	}

	if cfg.Dir == "" || cfg.StorePath == "" {
		return Config{}, fmt.Errorf("invalid config paths")
	}

	// Load optional config.toml preferences.
	loadConfigFile(&cfg)

	// DE_PROFILE env var overrides config file.
	if override := os.Getenv("DE_PROFILE"); override != "" {
		cfg.Profile = override
	}

	return cfg, nil
}

func Default() (Config, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return Config{}, err
	}

	dir := filepath.Join(base, AppName)
	return Config{
		Dir:       dir,
		StorePath: filepath.Join(dir, "store.json"),
		Color:     "auto",
		Profile:   DefaultProfile,
	}, nil
}

// ConfigFilePath returns the path to the optional config file.
func ConfigFilePath(cfg Config) string {
	return filepath.Join(cfg.Dir, "config.toml")
}

// loadConfigFile reads key=value pairs from config.toml if it exists.
func loadConfigFile(cfg *Config) {
	path := ConfigFilePath(*cfg)
	f, err := os.Open(path)
	if err != nil {
		return // missing file is fine
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' || line[0] == '[' {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")

		switch key {
		case "reveal":
			cfg.Reveal = value == "true" || value == "1"
		case "color":
			cfg.Color = value
		case "profile":
			if value != "" {
				cfg.Profile = value
			}
		case "store_path":
			if value != "" {
				cfg.StorePath = value
				cfg.Dir = filepath.Dir(value)
			}
		case "allow_reinit_on_decrypt_fail":
			cfg.AllowReinitOnDecryptFail = value == "true" || value == "1"
		}
	}
}

// SetProfileInFile updates or inserts the profile key in config.toml.
func SetProfileInFile(cfg Config, profile string) error {
	path := ConfigFilePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	// Read existing lines (file may not exist yet).
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(string(data), "\n")
	}

	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if k, _, ok := strings.Cut(trimmed, "="); ok && strings.TrimSpace(k) == "profile" {
			lines[i] = fmt.Sprintf("profile = %q", profile)
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("profile = %q", profile))
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600)
}
