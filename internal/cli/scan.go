package cli

import (
	"fmt"

	"github.com/Deadwood-cli/deadwood/internal/classify"
	"github.com/Deadwood-cli/deadwood/internal/git"
	"github.com/Deadwood-cli/deadwood/internal/output"
	"github.com/spf13/cobra"
)

// scanOptions holds flags specific to `deadwood scan`.
type scanOptions struct {
	json bool
}

const scanLong = `Scan classifies every local branch into one of five buckets: safe to delete,
squash-merged, needs review, active, or protected.

scan is strictly read-only. It never prompts for deletion and never modifies the
repository.

Until GitHub wiring lands, this is a local-only scan: remotes and pull requests
are not consulted, so no branch is reported as active or squash-merged.`

func newScanCommand(global *globalOptions) *cobra.Command {
	opts := &scanOptions{}

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Report which local branches are dead on the remote (read-only)",
		Long:  scanLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runScan(cmd, global, opts)
		},
	}

	cmd.Flags().BoolVar(&opts.json, "json", false,
		"emit machine-readable JSON instead of the human-readable report")

	return cmd
}

func runScan(cmd *cobra.Command, global *globalOptions, opts *scanOptions) error {
	report, err := buildScan(".", classify.DefaultConfig())
	if err != nil {
		return err
	}
	return output.Write(cmd.OutOrStdout(), report, output.Options{
		Verbose: global.verbose,
		JSON:    opts.json,
	})
}

// buildScan gathers local git metadata, classifies every branch, and returns a
// report. RemoteStatus and PRStatus are stubbed empty: phase 4 does not talk
// to remotes or GitHub, so Exists is false and no PR is ever matched.
func buildScan(repoPath string, cfg classify.Config) (output.Report, error) {
	root, err := git.RepoRoot(repoPath)
	if err != nil {
		return output.Report{}, fmt.Errorf("not a git repository: %w", err)
	}

	defaultBranch, err := git.GetDefaultBranch(root)
	if err != nil {
		return output.Report{}, fmt.Errorf(
			"cannot determine the default branch: %w (run `git remote set-head origin -a` or wait for GitHub detection)",
			err,
		)
	}

	locals, err := git.ListLocalBranches(root)
	if err != nil {
		return output.Report{}, err
	}

	worktrees, err := git.ListWorktrees(root)
	if err != nil {
		return output.Report{}, err
	}
	inWorktree := make(map[string]bool, len(worktrees))
	for _, name := range worktrees {
		inWorktree[name] = true
	}

	names := make([]string, len(locals))
	for i, branch := range locals {
		names[i] = branch.Name
	}
	stashes, err := git.ListStashRefs(root)
	if err != nil {
		return output.Report{}, err
	}
	stashed := git.BranchesWithStash(stashes, names)

	results := make([]classify.BranchResult, 0, len(locals))
	for _, local := range locals {
		info, err := classifyInfo(root, local, defaultBranch, inWorktree[local.Name], stashed[local.Name])
		if err != nil {
			return output.Report{}, err
		}
		results = append(results, classify.Classify(info, classify.RemoteStatus{}, classify.PRStatus{}, defaultBranch, cfg))
	}

	return output.Report{
		DefaultBranch: defaultBranch,
		LocalOnly:     true,
		Results:       results,
	}, nil
}

func classifyInfo(repoPath string, local git.BranchInfo, defaultBranch string, hasWorktree, hasStash bool) (classify.BranchInfo, error) {
	info := classify.BranchInfo{
		Name:           local.Name,
		IsCurrent:      local.IsCurrent,
		HasWorktree:    hasWorktree,
		HasStash:       hasStash,
		LastCommitSHA:  local.LastCommitSHA,
		LastCommitDate: local.LastCommitDate,
		HasUpstream:    local.HasUpstream,
		UpstreamName:   local.UpstreamName,
	}

	// Protected checks do not read these fields, so skip the extra git calls
	// for branches that cannot be deleted anyway.
	if local.IsCurrent || hasWorktree || hasStash {
		return info, nil
	}

	ancestor, err := git.IsAncestor(repoPath, local.Name, defaultBranch)
	if err != nil {
		return classify.BranchInfo{}, fmt.Errorf("checking whether %q is merged into %s: %w", local.Name, defaultBranch, err)
	}
	ahead, behind, err := git.AheadBehind(repoPath, local.Name, defaultBranch)
	if err != nil {
		return classify.BranchInfo{}, fmt.Errorf("counting commits on %q relative to %s: %w", local.Name, defaultBranch, err)
	}

	info.IsAncestor = ancestor
	info.AheadCount = ahead
	info.BehindCount = behind
	return info, nil
}
