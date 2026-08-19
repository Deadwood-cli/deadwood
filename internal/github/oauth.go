package github

import (
	"fmt"
	"os"
	"strings"
)

const (
	// EnvClientID overrides the compiled OAuth client ID (useful in tests or
	// when pointing a private build at a different OAuth App).
	EnvClientID = "DEADWOOD_GITHUB_CLIENT_ID"

	// Scope is the OAuth scope requested by device flow. Deadwood only reads
	// the API in v0.1; it never writes to a GitHub repo.
	Scope = "repo"
)

// OAuthClientID is the public GitHub OAuth App client ID for the Deadwood-cli
// org app. Device flow does not use a client secret. This is not a credential.
const OAuthClientID = "Ov23li4w4WBJxsCJzb07"

// ResolveClientID returns the public OAuth App client ID, preferring
// DEADWOOD_GITHUB_CLIENT_ID over the compiled constant.
func ResolveClientID() (string, error) {
	if id := strings.TrimSpace(os.Getenv(EnvClientID)); id != "" {
		return id, nil
	}
	if id := strings.TrimSpace(OAuthClientID); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("no GitHub OAuth client ID: register an OAuth App under the Deadwood-cli org (device flow, no client secret) and set %s, or bake the client ID into OAuthClientID", EnvClientID)
}
