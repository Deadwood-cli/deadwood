package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultsMatchSpecSection9(t *testing.T) {
	cfg := Defaults()

	assert.Equal(t, []string{"main", "master", "develop", "release/*", "hotfix/*"}, cfg.ExcludePatterns)
	assert.Equal(t, 90, cfg.StaleWarningDays)
	assert.Equal(t, 30, cfg.BackupRetentionDays)
	assert.Empty(t, cfg.DefaultBranch)
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	got, err := Load(t.TempDir(), "")
	require.NoError(t, err)
	assert.Equal(t, Defaults(), got.Config)
	assert.Empty(t, got.Path)
}

func TestLoadTable(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    Config
		wantErr string
	}{
		{
			name: "full spec example",
			body: `
exclude_patterns:
  - "main"
  - "master"
  - "develop"
  - "release/*"
  - "hotfix/*"
stale_warning_days: 90
default_branch: ""
backup_retention_days: 30
`,
			want: Defaults(),
		},
		{
			name: "custom exclude replaces defaults",
			body: `
exclude_patterns:
  - "trunk"
  - "feat/*"
default_branch: trunk
`,
			want: Config{
				ExcludePatterns:     []string{"trunk", "feat/*"},
				StaleWarningDays:    90,
				DefaultBranch:       "trunk",
				BackupRetentionDays: 30,
			},
		},
		{
			name: "omitted exclude_patterns keeps defaults",
			body: "default_branch: develop\n",
			want: Config{
				ExcludePatterns:     Defaults().ExcludePatterns,
				StaleWarningDays:    90,
				DefaultBranch:       "develop",
				BackupRetentionDays: 30,
			},
		},
		{
			name: "explicit empty exclude list is empty",
			body: "exclude_patterns: []\n",
			want: Config{
				ExcludePatterns:     []string{},
				StaleWarningDays:    90,
				BackupRetentionDays: 30,
			},
		},
		{
			name:    "malformed yaml",
			body:    "exclude_patterns: [\n",
			wantErr: "malformed config file",
		},
		{
			name:    "unknown field",
			body:    "exclude_pattern: [main]\n",
			wantErr: "malformed config file",
		},
		{
			name:    "wrong type",
			body:    "stale_warning_days: ninety\n",
			wantErr: "malformed config file",
		},
		{
			name:    "invalid glob",
			body:    "exclude_patterns: [\"feat[\"]\n",
			wantErr: "invalid exclude pattern",
		},
		{
			name:    "negative stale_warning_days",
			body:    "stale_warning_days: -1\n",
			wantErr: "stale_warning_days",
		},
		{
			name: "empty file is defaults",
			body: "",
			want: Defaults(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, FileName)
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))

			got, err := Load(dir, "")
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Contains(t, err.Error(), path)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Config)
			assert.Equal(t, path, got.Path)
		})
	}
}

func TestLoadExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yml")
	require.NoError(t, os.WriteFile(path, []byte("default_branch: trunk\n"), 0o600))

	got, err := Load(t.TempDir(), path)
	require.NoError(t, err)
	assert.Equal(t, "trunk", got.Config.DefaultBranch)
	assert.Equal(t, Defaults().ExcludePatterns, got.Config.ExcludePatterns)
	assert.Equal(t, path, got.Path)
}

func TestLoadExplicitPathMissingFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yml")

	_, err := Load(t.TempDir(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file")
	assert.Contains(t, err.Error(), path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestClassifyCopiesExcludePatterns(t *testing.T) {
	cfg := Defaults()
	out := cfg.Classify()
	out.ExcludePatterns[0] = "mutated"
	assert.Equal(t, "main", cfg.ExcludePatterns[0])
}
