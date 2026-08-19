package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveClientIDUsesCompiledID(t *testing.T) {
	t.Setenv(EnvClientID, "")

	id, err := ResolveClientID()

	require.NoError(t, err)
	assert.Equal(t, OAuthClientID, id)
	assert.NotEmpty(t, id, "the compiled client ID must be set once the OAuth App exists")
}

func TestResolveClientIDPrefersEnv(t *testing.T) {
	t.Setenv(EnvClientID, "env-override-id")

	id, err := ResolveClientID()

	require.NoError(t, err)
	assert.Equal(t, "env-override-id", id)
}
