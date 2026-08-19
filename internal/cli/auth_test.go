package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Deadwood-cli/deadwood/internal/auth"
	ghub "github.com/Deadwood-cli/deadwood/internal/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runAuth(t *testing.T, runtime *authRuntime, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	root := newRootCommand(testBuildInfo(), runtime, nil)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{}, args...))
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func TestAuthStatusNotLoggedIn(t *testing.T) {
	stdout, _, err := runAuth(t, mockAuthRuntime(nil), "auth", "status")

	require.NoError(t, err)
	assert.Equal(t, "not logged in\n", stdout)
}

func TestAuthStatusFromKeychain(t *testing.T) {
	runtime := mockAuthRuntime(nil)
	require.NoError(t, runtime.store.Set("stored-token"))
	runtime.login = func(context.Context, string) (string, error) { return "octocat", nil }

	stdout, stderr, err := runAuth(t, runtime, "auth", "status")

	require.NoError(t, err)
	assert.Empty(t, stderr)
	assert.Equal(t, "logged in as octocat (OS keychain)\n", stdout)
}

func TestAuthStatusFromEnvWarns(t *testing.T) {
	runtime := mockAuthRuntime(map[string]string{auth.EnvToken: "env-token"})
	runtime.login = func(_ context.Context, token string) (string, error) {
		assert.Equal(t, "env-token", token)
		return "octocat", nil
	}

	stdout, stderr, err := runAuth(t, runtime, "auth", "status")

	require.NoError(t, err)
	assert.Contains(t, stderr, "warning: using "+auth.EnvToken)
	assert.Equal(t, "logged in as octocat ("+auth.EnvToken+")\n", stdout)
}

func TestAuthLogout(t *testing.T) {
	runtime := mockAuthRuntime(nil)
	require.NoError(t, runtime.store.Set("stored-token"))

	stdout, _, err := runAuth(t, runtime, "auth", "logout")

	require.NoError(t, err)
	assert.Equal(t, "logged out\n", stdout)
	_, err = runtime.store.Get()
	assert.ErrorIs(t, err, auth.ErrNotFound)
}

func TestAuthLoginDeviceFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "device-abc",
			"user_code":        "WDJB-MJHT",
			"verification_uri": "https://github.com/login/device",
			"expires_in":       900,
			"interval":         1,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "gho-test-not-a-real-token"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	flow := ghub.NewDeviceFlow(srv.Client())
	flow.CodeURL = srv.URL + "/device/code"
	flow.TokenURL = srv.URL + "/token"
	flow.Sleep = func(time.Duration) {}

	runtime := mockAuthRuntime(nil)
	runtime.flow = flow
	runtime.login = func(_ context.Context, token string) (string, error) {
		assert.Equal(t, "gho-test-not-a-real-token", token)
		return "octocat", nil
	}

	stdout, stderr, err := runAuth(t, runtime, "auth", "login")

	require.NoError(t, err)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "https://github.com/login/device")
	assert.Contains(t, stdout, "WDJB-MJHT")
	assert.Contains(t, stdout, "logged in as octocat")
	assert.False(t, strings.Contains(stdout, "gho-test-not-a-real-token"))
	assert.False(t, strings.Contains(stderr, "gho-test-not-a-real-token"))

	got, err := runtime.store.Get()
	require.NoError(t, err)
	assert.Equal(t, "gho-test-not-a-real-token", got.Value)
}

func TestAuthLoginOutputNeverIncludesToken(t *testing.T) {
	runtime := mockAuthRuntime(nil)
	runtime.clientID = func() (string, error) { return "", assert.AnError }

	stdout, stderr, err := runAuth(t, runtime, "auth", "login")

	require.Error(t, err)
	assert.NotContains(t, stdout+stderr+err.Error(), "gho-")
}

func mockAuthRuntime(env map[string]string) *authRuntime {
	lookup := func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
	return &authRuntime{
		store:     auth.NewStore(auth.NewMemoryRing(), lookup),
		flow:      ghub.NewDeviceFlow(nil),
		login:     func(context.Context, string) (string, error) { return "", nil },
		clientID:  func() (string, error) { return "test-client", nil },
		lookupEnv: lookup,
	}
}
