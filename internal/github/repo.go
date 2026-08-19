package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v63/github"
)

// RepoDefaultBranch returns the repository's default branch from GitHub,
// the second source in spec Section 5.2 after origin/HEAD.
func RepoDefaultBranch(ctx context.Context, client *gh.Client, repo Repo) (string, error) {
	info, _, err := client.Repositories.Get(ctx, repo.Owner, repo.Name)
	if err != nil {
		return "", fmt.Errorf("reading default branch from GitHub: %w", err)
	}
	name := info.GetDefaultBranch()
	if name == "" {
		return "", fmt.Errorf("GitHub repository %s/%s has an empty default_branch", repo.Owner, repo.Name)
	}
	return name, nil
}
