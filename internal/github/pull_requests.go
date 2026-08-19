package github

import (
	"context"
	"fmt"
	"time"

	gh "github.com/google/go-github/v63/github"
)

// PRMatch is the squash-merge evidence Classify needs. It is a github-package
// type so this package does not import classify.
type PRMatch struct {
	Found    bool
	Merged   bool
	Number   int
	MergedAt time.Time
}

// perBranchHeadBudget is how many extra core calls we keep in reserve on top
// of one call per branch (rate-limit check, default-branch lookup, etc.).
const perBranchHeadBudget = 5

// MergedPRs looks up merged pull requests whose head ref matches each branch
// name. Results are cached in the returned map for the rest of the process.
//
// When the remaining core rate-limit budget is too small for one request per
// branch, this lists closed PRs once (paginated) and matches head.ref locally
// instead (spec Section 5.3).
func MergedPRs(ctx context.Context, client *gh.Client, repo Repo, branches []string) (map[string]PRMatch, error) {
	out := make(map[string]PRMatch, len(branches))
	if len(branches) == 0 {
		return out, nil
	}

	remaining, err := CoreRemaining(ctx, client)
	if err != nil || remaining < len(branches)+perBranchHeadBudget {
		return matchFromClosedList(ctx, client, repo, branches)
	}

	for _, branch := range branches {
		match, err := mergedPRByHead(ctx, client, repo, branch)
		if err != nil {
			return nil, err
		}
		out[branch] = match
	}
	return out, nil
}

func mergedPRByHead(ctx context.Context, client *gh.Client, repo Repo, branch string) (PRMatch, error) {
	opts := &gh.PullRequestListOptions{
		State: "closed",
		Head:  repo.Owner + ":" + branch,
		ListOptions: gh.ListOptions{
			PerPage: 10,
		},
	}
	prs, _, err := client.PullRequests.List(ctx, repo.Owner, repo.Name, opts)
	if err != nil {
		return PRMatch{}, fmt.Errorf("listing pull requests for %s: %w", branch, err)
	}
	return bestMatch(prs, branch), nil
}

func matchFromClosedList(ctx context.Context, client *gh.Client, repo Repo, branches []string) (map[string]PRMatch, error) {
	wanted := make(map[string]struct{}, len(branches))
	for _, branch := range branches {
		wanted[branch] = struct{}{}
	}

	opts := &gh.PullRequestListOptions{
		State: "closed",
		ListOptions: gh.ListOptions{
			PerPage: 100,
		},
	}
	byHead := make(map[string][]*gh.PullRequest)
	for {
		prs, resp, err := client.PullRequests.List(ctx, repo.Owner, repo.Name, opts)
		if err != nil {
			return nil, fmt.Errorf("listing closed pull requests: %w", err)
		}
		for _, pr := range prs {
			ref := pr.GetHead().GetRef()
			if _, ok := wanted[ref]; ok {
				byHead[ref] = append(byHead[ref], pr)
			}
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	out := make(map[string]PRMatch, len(branches))
	for _, branch := range branches {
		out[branch] = bestMatch(byHead[branch], branch)
	}
	return out, nil
}

func bestMatch(prs []*gh.PullRequest, branch string) PRMatch {
	var best PRMatch
	for _, pr := range prs {
		if branch != "" && pr.GetHead().GetRef() != branch {
			continue
		}
		match := PRMatch{Found: true, Number: pr.GetNumber()}
		if mergedAt := pr.GetMergedAt(); !mergedAt.IsZero() {
			match.Merged = true
			match.MergedAt = mergedAt.Time
		}
		if !best.Found || (match.Merged && !best.Merged) || (match.Merged && match.MergedAt.After(best.MergedAt)) {
			best = match
		}
	}
	return best
}
