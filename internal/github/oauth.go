package github

import (
	"fmt"
	"os"
	"strings"
)

const (
	// EnvClientID overrides the compiled OAuth client ID, so login can be
	// tested before the org OAuth App is registered.
	EnvClientID = "DEADWOOD_GITHUB_CLIENT_ID"

	// Scope is the OAuth scope requested by device flow. Deadwood only reads
	// the API in v0.1; it never writes to a GitHub repo.
	Scope = "repo"
)

// OAuthClientID is the public GitHub OAuth App client ID. Device flow does not
// use a client secret. Leave empty until the app is registered under the
// Deadwood-cli org; login then requires DEADWOOD_GITHUB_CLIENT_ID.
const OAuthClientID = ""

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
