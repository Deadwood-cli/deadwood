// Package github talks to GitHub's HTTP APIs. Auth tokens arrive from
// internal/auth; this package never stores them.
package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	gh "github.com/google/go-github/v63/github"
)

// httpTimeout bounds GitHub API calls. Device flow uses its own client.
const httpTimeout = 30 * time.Second

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: httpTimeout}
}

// ClientOptions configure a go-github client. BaseURL is for tests against
// httptest.Server; production leaves it empty.
type ClientOptions struct {
	HTTP    *http.Client
	BaseURL string
}

// NewClient returns an authenticated GitHub API client. Deadwood never uses
// this client to write to a repository.
func NewClient(token string, opts ClientOptions) *gh.Client {
	httpClient := opts.HTTP
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}
	client := gh.NewClient(httpClient).WithAuthToken(token)
	if opts.BaseURL != "" {
		base, err := url.Parse(opts.BaseURL)
		if err == nil {
			if base.Path == "" || base.Path[len(base.Path)-1] != '/' {
				base.Path += "/"
			}
			client.BaseURL = base
		}
	}
	return client
}

// AuthenticatedLogin calls GET /user and returns the login name. A 401 is
// reported as an invalid token without leaking the token value.
func AuthenticatedLogin(ctx context.Context, token string, opts ClientOptions) (string, error) {
	user, resp, err := NewClient(token, opts).Users.Get(ctx, "")
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("GitHub token is invalid or expired")
		}
		return "", fmt.Errorf("validating GitHub token: %w", err)
	}
	if user.GetLogin() == "" {
		return "", fmt.Errorf("GitHub GET /user returned an empty login")
	}
	return user.GetLogin(), nil
}
