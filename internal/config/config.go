// Package config loads .deadwood.yml. A missing file is Defaults(); a
// malformed file is an error — never a silent fallback (spec Section 9).
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/Deadwood-cli/deadwood/internal/classify"
	"gopkg.in/yaml.v3"
)

// Config is the parsed .deadwood.yml. StaleWarningDays and BackupRetentionDays
// are stored for later phases; they do not change bucket assignment in v0.1.
type Config struct {
	ExcludePatterns     []string
	StaleWarningDays    int
	DefaultBranch       string
	BackupRetentionDays int
}

// Classify returns the subset of Config the decision tree reads.
func (c Config) Classify() classify.Config {
	patterns := make([]string, len(c.ExcludePatterns))
	copy(patterns, c.ExcludePatterns)
	return classify.Config{ExcludePatterns: patterns}
}

// file is the on-disk shape. Pointers distinguish "omitted, use defaults"
// from an explicit empty list or zero.
type file struct {
	ExcludePatterns     *[]string `yaml:"exclude_patterns"`
	StaleWarningDays    *int      `yaml:"stale_warning_days"`
	DefaultBranch       *string   `yaml:"default_branch"`
	BackupRetentionDays *int      `yaml:"backup_retention_days"`
}

// LoadResult is a parsed config plus the file it came from.
type LoadResult struct {
	// Config is the effective configuration after defaults and the file are applied.
	Config Config
	// Path is the file that was read. Empty means built-in Defaults() because
	// no .deadwood.yml existed at the repo root.
	Path string
}

// Load reads configuration for a repository. If explicitPath is set (--config),
// that file is required. Otherwise .deadwood.yml at repoRoot is used when it
// exists, and Defaults() when it does not.
func Load(repoRoot, explicitPath string) (LoadResult, error) {
	filePath := explicitPath
	if filePath == "" {
		filePath = filepath.Join(repoRoot, FileName)
		_, err := os.Stat(filePath)
		if errors.Is(err, os.ErrNotExist) {
			return LoadResult{Config: Defaults()}, nil
		}
		if err != nil {
			return LoadResult{}, fmt.Errorf("stat config file %s: %w", filePath, err)
		}
	}

	cfg, err := loadFile(filePath)
	if err != nil {
		return LoadResult{}, err
	}
	return LoadResult{Config: cfg, Path: filePath}, nil
}

func loadFile(filePath string) (Config, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return Config{}, fmt.Errorf("config file %s: %w", filePath, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var raw file
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return Defaults(), nil
		}
		return Config{}, fmt.Errorf("malformed config file %s: %w", filePath, err)
	}

	cfg, err := applyFile(raw)
	if err != nil {
		return Config{}, fmt.Errorf("malformed config file %s: %w", filePath, err)
	}
	return cfg, nil
}

func applyFile(raw file) (Config, error) {
	cfg := Defaults()
	if raw.ExcludePatterns != nil {
		cfg.ExcludePatterns = *raw.ExcludePatterns
	}
	if raw.StaleWarningDays != nil {
		cfg.StaleWarningDays = *raw.StaleWarningDays
	}
	if raw.DefaultBranch != nil {
		cfg.DefaultBranch = *raw.DefaultBranch
	}
	if raw.BackupRetentionDays != nil {
		cfg.BackupRetentionDays = *raw.BackupRetentionDays
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validate(cfg Config) error {
	if cfg.StaleWarningDays < 0 {
		return fmt.Errorf("stale_warning_days must be >= 0, got %d", cfg.StaleWarningDays)
	}
	if cfg.BackupRetentionDays < 0 {
		return fmt.Errorf("backup_retention_days must be >= 0, got %d", cfg.BackupRetentionDays)
	}
	for _, pattern := range cfg.ExcludePatterns {
		if _, err := path.Match(pattern, ""); err != nil {
			return fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
	}
	return nil
}
