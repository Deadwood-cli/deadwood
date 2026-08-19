package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOriginURL(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw     string
		want    Repo
		wantErr error
	}{
		"https": {
			raw:  "https://github.com/Deadwood-cli/deadwood.git",
			want: Repo{Owner: "Deadwood-cli", Name: "deadwood"},
		},
		"https without git suffix": {
			raw:  "https://github.com/Deadwood-cli/deadwood",
			want: Repo{Owner: "Deadwood-cli", Name: "deadwood"},
		},
		"ssh scp": {
			raw:  "git@github.com:Deadwood-cli/deadwood.git",
			want: Repo{Owner: "Deadwood-cli", Name: "deadwood"},
		},
		"ssh url": {
			raw:  "ssh://git@github.com/Deadwood-cli/deadwood.git",
			want: Repo{Owner: "Deadwood-cli", Name: "deadwood"},
		},
		"host is case-insensitive": {
			raw:  "https://GitHub.com/Deadwood-cli/deadwood.git",
			want: Repo{Owner: "Deadwood-cli", Name: "deadwood"},
		},
		"gitlab is rejected": {
			raw:     "https://gitlab.com/org/repo.git",
			wantErr: ErrNotGitHub,
		},
		"ssh gitlab is rejected": {
			raw:     "git@gitlab.com:org/repo.git",
			wantErr: ErrNotGitHub,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseOriginURL(tc.raw)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
