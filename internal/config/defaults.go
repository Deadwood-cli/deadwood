package config

import "github.com/Deadwood-cli/deadwood/internal/classify"

// FileName is the default config file at the repository root (spec Section 9).
const FileName = ".deadwood.yml"

const (
	defaultStaleWarningDays    = 90
	defaultBackupRetentionDays = 30
)

// Defaults is the configuration used when no .deadwood.yml is present.
// Exclude patterns match spec Section 9 so a missing file still protects
// main/master/develop and the release/hotfix globs.
func Defaults() Config {
	return Config{
		ExcludePatterns:     classify.DefaultConfig().ExcludePatterns,
		StaleWarningDays:    defaultStaleWarningDays,
		BackupRetentionDays: defaultBackupRetentionDays,
	}
}
