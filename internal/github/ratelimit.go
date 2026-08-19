package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v63/github"
)

// CoreRemaining returns how many REST core API calls the token can still
// make. A failed lookup returns 0 remaining so callers fall back to the
// cheaper bulk PR list (spec Section 5.3).
func CoreRemaining(ctx context.Context, client *gh.Client) (int, error) {
	limits, _, err := client.RateLimit.Get(ctx)
	if err != nil {
		return 0, fmt.Errorf("reading GitHub rate limit: %w", err)
	}
	if limits == nil || limits.Core == nil {
		return 0, nil
	}
	return limits.Core.Remaining, nil
}
