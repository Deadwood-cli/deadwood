package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestCode(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "client_id=test-client")
		assert.Contains(t, string(body), "scope=repo")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "device-abc",
			"user_code":        "WDJB-MJHT",
			"verification_uri": "https://github.com/login/device",
			"expires_in":       900,
			"interval":         5,
		})
	}))
	t.Cleanup(srv.Close)

	flow := NewDeviceFlow(srv.Client())
	flow.CodeURL = srv.URL

	code, err := flow.RequestCode(context.Background(), "test-client")

	require.NoError(t, err)
	assert.Equal(t, "device-abc", code.DeviceCode)
	assert.Equal(t, "WDJB-MJHT", code.UserCode)
	assert.Equal(t, "https://github.com/login/device", code.VerificationURI)
	assert.Equal(t, 5*time.Second, code.Interval)
}

func TestWaitForTokenPendingThenSuccess(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "gho-test-not-a-real-token"})
	}))
	t.Cleanup(srv.Close)

	var slept []time.Duration
	flow := NewDeviceFlow(srv.Client())
	flow.TokenURL = srv.URL
	flow.Sleep = func(d time.Duration) { slept = append(slept, d) }

	token, err := flow.WaitForToken(context.Background(), "test-client", DeviceCode{
		DeviceCode: "device-abc",
		ExpiresIn:  time.Minute,
		Interval:   2 * time.Second,
	})

	require.NoError(t, err)
	assert.Equal(t, "gho-test-not-a-real-token", token)
	assert.Equal(t, []time.Duration{2 * time.Second}, slept)
	assert.Equal(t, int32(2), calls.Load())
}

func TestWaitForTokenSlowDown(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "gho-test-not-a-real-token"})
	}))
	t.Cleanup(srv.Close)

	var slept []time.Duration
	flow := NewDeviceFlow(srv.Client())
	flow.TokenURL = srv.URL
	flow.Sleep = func(d time.Duration) { slept = append(slept, d) }

	token, err := flow.WaitForToken(context.Background(), "test-client", DeviceCode{
		DeviceCode: "device-abc",
		ExpiresIn:  time.Minute,
		Interval:   5 * time.Second,
	})

	require.NoError(t, err)
	assert.Equal(t, "gho-test-not-a-real-token", token)
	require.NotEmpty(t, slept)
	assert.Equal(t, 10*time.Second, slept[0])
}

func TestWaitForTokenAccessDenied(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "access_denied"})
	}))
	t.Cleanup(srv.Close)

	flow := NewDeviceFlow(srv.Client())
	flow.TokenURL = srv.URL
	flow.Sleep = func(time.Duration) {}

	_, err := flow.WaitForToken(context.Background(), "test-client", DeviceCode{
		DeviceCode: "device-abc",
		ExpiresIn:  time.Minute,
		Interval:   time.Millisecond,
	})

	assert.ErrorIs(t, err, ErrAccessDenied)
}

func TestWaitForTokenHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream exploded"))
	}))
	t.Cleanup(srv.Close)

	flow := NewDeviceFlow(srv.Client())
	flow.TokenURL = srv.URL
	flow.Sleep = func(time.Duration) {}

	_, err := flow.WaitForToken(context.Background(), "test-client", DeviceCode{
		DeviceCode: "device-abc",
		ExpiresIn:  time.Minute,
		Interval:   time.Millisecond,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestWaitForTokenExpired(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "expired_token"})
	}))
	t.Cleanup(srv.Close)

	flow := NewDeviceFlow(srv.Client())
	flow.TokenURL = srv.URL
	flow.Sleep = func(time.Duration) {}

	_, err := flow.WaitForToken(context.Background(), "test-client", DeviceCode{
		DeviceCode: "device-abc",
		ExpiresIn:  time.Minute,
		Interval:   time.Millisecond,
	})

	assert.ErrorIs(t, err, ErrDeviceCodeExpired)
}
