package github

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrNotGitHub reports that origin does not point at github.com. v0.1 does
// not silently no-op on other hosts. The CLI prints the spec Section 7.2
// sentence when this error is returned.
var ErrNotGitHub = errors.New("origin remote is not GitHub")

// Repo is the owner/name pair GitHub's API uses.
type Repo struct {
	Owner string
	Name  string
}

// ParseOriginURL extracts owner/name from an origin remote URL. SSH
// (git@github.com:owner/repo.git) and HTTPS (https://github.com/owner/repo.git)
// forms are both accepted. Host matching is case-insensitive.
func ParseOriginURL(raw string) (Repo, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Repo{}, fmt.Errorf("origin URL is empty")
	}

	if strings.HasPrefix(raw, "git@") {
		return parseSCP(raw)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return Repo{}, fmt.Errorf("parsing origin URL: %w", err)
	}

	host := strings.ToLower(parsed.Hostname())
	if host != "github.com" && host != "www.github.com" {
		return Repo{}, ErrNotGitHub
	}

	path := strings.TrimPrefix(parsed.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	return splitOwnerName(path)
}

func parseSCP(raw string) (Repo, error) {
	// git@github.com:owner/repo.git
	_, rest, ok := strings.Cut(raw, "@")
	if !ok {
		return Repo{}, fmt.Errorf("parsing SSH origin URL %q", raw)
	}
	host, path, ok := strings.Cut(rest, ":")
	if !ok {
		return Repo{}, fmt.Errorf("parsing SSH origin URL %q: missing colon", raw)
	}
	if strings.ToLower(host) != "github.com" {
		return Repo{}, ErrNotGitHub
	}
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	return splitOwnerName(path)
}

func splitOwnerName(path string) (Repo, error) {
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Repo{}, fmt.Errorf("origin URL path %q is not owner/repo", path)
	}
	return Repo{Owner: parts[0], Name: parts[1]}, nil
}
