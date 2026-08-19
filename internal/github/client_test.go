package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedLogin(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/user", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"login": "octocat"})
	}))
	t.Cleanup(srv.Close)

	login, err := AuthenticatedLogin(context.Background(), "test-token", ClientOptions{
		HTTP:    srv.Client(),
		BaseURL: srv.URL,
	})

	require.NoError(t, err)
	assert.Equal(t, "octocat", login)
}

func TestAuthenticatedLoginUnauthorized(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := AuthenticatedLogin(context.Background(), "test-token", ClientOptions{
		HTTP:    srv.Client(),
		BaseURL: srv.URL,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired")
	assert.NotContains(t, err.Error(), "test-token")
}
