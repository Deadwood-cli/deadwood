package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestGetPrefersEnvOverKeychain(t *testing.T) {
	t.Parallel()

	ring := NewMemoryRing()
	require.NoError(t, ring.Set(Service, Account, "from-keychain"))
	store := NewStore(ring, func(key string) (string, bool) {
		if key == EnvToken {
			return "from-env", true
		}
		return "", false
	})

	got, err := store.Get()

	require.NoError(t, err)
	assert.Equal(t, "from-env", got.Value)
	assert.True(t, got.FromEnv)
}

func TestGetFallsBackToKeychain(t *testing.T) {
	t.Parallel()

	ring := NewMemoryRing()
	require.NoError(t, ring.Set(Service, Account, "from-keychain"))
	store := NewStore(ring, func(string) (string, bool) { return "", false })

	got, err := store.Get()

	require.NoError(t, err)
	assert.Equal(t, "from-keychain", got.Value)
	assert.False(t, got.FromEnv)
}

func TestGetIgnoresEmptyEnvVar(t *testing.T) {
	t.Parallel()

	ring := NewMemoryRing()
	require.NoError(t, ring.Set(Service, Account, "from-keychain"))
	store := NewStore(ring, func(key string) (string, bool) {
		if key == EnvToken {
			return "", true
		}
		return "", false
	})

	got, err := store.Get()

	require.NoError(t, err)
	assert.Equal(t, "from-keychain", got.Value)
	assert.False(t, got.FromEnv)
}

func TestGetNotFound(t *testing.T) {
	t.Parallel()

	store := NewStore(NewMemoryRing(), func(string) (string, bool) { return "", false })

	_, err := store.Get()

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSetAndDelete(t *testing.T) {
	t.Parallel()

	store := NewStore(NewMemoryRing(), func(string) (string, bool) { return "", false })

	require.NoError(t, store.Set("stored-token"))
	got, err := store.Get()
	require.NoError(t, err)
	assert.Equal(t, "stored-token", got.Value)

	require.NoError(t, store.Delete())
	_, err = store.Get()
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSetRejectsEmptyToken(t *testing.T) {
	t.Parallel()

	err := NewStore(NewMemoryRing(), nil).Set("")

	assert.Error(t, err)
}

func TestDeleteMissingEntry(t *testing.T) {
	t.Parallel()

	err := NewStore(NewMemoryRing(), nil).Delete()

	assert.NoError(t, err)
}

func TestMemoryRingMatchesKeyringNotFound(t *testing.T) {
	t.Parallel()

	_, err := NewMemoryRing().Get(Service, Account)

	assert.ErrorIs(t, err, keyring.ErrNotFound)
}
