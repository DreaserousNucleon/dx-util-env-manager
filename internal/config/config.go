package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const AppName = "de"

type Config struct {
	Dir       string
	StorePath string
	// User preferences (from config.toml).
	Reveal                   bool   // default reveal mode
	Color                    string // "auto", "always", "never"
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
