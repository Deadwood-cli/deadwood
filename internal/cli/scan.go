package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/Deadwood-cli/deadwood/internal/auth"
	"github.com/Deadwood-cli/deadwood/internal/classify"
	"github.com/Deadwood-cli/deadwood/internal/config"
	"github.com/Deadwood-cli/deadwood/internal/git"
	ghub "github.com/Deadwood-cli/deadwood/internal/github"
	"github.com/Deadwood-cli/deadwood/internal/output"
	"github.com/spf13/cobra"
)

// scanOptions holds flags specific to `deadwood scan`.
type scanOptions struct {
	json bool
}

type scanDeps struct {
	originURL   func(repoPath string) (string, error)
	listRemotes func(repoPath string) (map[string]struct{}, error)
	store       *auth.Store
	clientOpts  ghub.ClientOptions
	mergedPRs   func(ctx context.Context, token string, repo ghub.Repo, branches []string) (map[string]ghub.PRMatch, error)
	ghDefault   func(ctx context.Context, token string, repo ghub.Repo) (string, error)
}

func defaultScanDeps() *scanDeps {
	opts := ghub.ClientOptions{}
	return &scanDeps{
		originURL:   git.OriginURL,
		listRemotes: git.ListRemoteBranches,
		store:       auth.DefaultStore(),
		clientOpts:  opts,
		mergedPRs: func(ctx context.Context, token string, repo ghub.Repo, branches []string) (map[string]ghub.PRMatch, error) {
			return ghub.MergedPRs(ctx, ghub.NewClient(token, opts), repo, branches)
		},
		ghDefault: func(ctx context.Context, token string, repo ghub.Repo) (string, error) {
			return ghub.RepoDefaultBranch(ctx, ghub.NewClient(token, opts), repo)
		},
	}
}

const scanLong = `Scan classifies every local branch into one of five buckets: safe to delete,
squash-merged, needs review, active, or protected.

scan is strictly read-only. It never prompts for deletion and never modifies the
repository.`

func newScanCommand(global *globalOptions, deps *scanDeps) *cobra.Command {
	if deps == nil {
		deps = defaultScanDeps()
	}
	opts := &scanOptions{}

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Report which local branches are dead on the remote (read-only)",
		Long:  scanLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runScan(cmd, global, opts, deps)
		},
	}

	cmd.Flags().BoolVar(&opts.json, "json", false,
		"emit machine-readable JSON (name, bucket, reason, confidence, commit) instead of the human report")

	return cmd
}

func runScan(cmd *cobra.Command, global *globalOptions, opts *scanOptions, deps *scanDeps) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	root, err := git.RepoRoot(".")
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	loaded, err := config.Load(root, global.configPath)
	if err != nil {
		return err
	}
	cfg := loaded.Config
	if len(cfg.ExcludePatterns) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: exclude_patterns is empty; default protected names (main, master, ...) are not excluded")
	}
	report, err := buildScan(ctx, root, cfg, deps, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	return output.Write(cmd.OutOrStdout(), report, output.Options{
		Verbose: global.verbose,
		JSON:    opts.json,
	})
}

func buildScan(ctx context.Context, repoPath string, cfg config.Config, deps *scanDeps, warnings io.Writer) (output.Report, error) {
	return scanRepo(ctx, repoPath, cfg, deps, warnings, nil)
}

func scanRepo(ctx context.Context, repoPath string, cfg config.Config, deps *scanDeps, warnings io.Writer, only []string) (output.Report, error) {
	if deps == nil {
		deps = defaultScanDeps()
	}

	root, err := git.RepoRoot(repoPath)
	if err != nil {
		return output.Report{}, fmt.Errorf("not a git repository: %w", err)
	}

	origin, err := deps.originURL(root)
	if err != nil {
		return output.Report{}, fmt.Errorf("reading origin URL: %w", err)
	}
	repo, err := ghub.ParseOriginURL(origin)
	if err != nil {
		if errors.Is(err, ghub.ErrNotGitHub) {
			return output.Report{}, fmt.Errorf("v0.1 only supports GitHub; GitLab support is planned: %w", err)
		}
		return output.Report{}, err
	}

	classCfg := cfg.Classify()
	defaultBranch, err := resolveDefaultBranch(ctx, root, repo, deps, warnings, cfg.DefaultBranch)
	if err != nil {
		return output.Report{}, err
	}

	locals, err := git.ListLocalBranches(root)
	if err != nil {
		return output.Report{}, err
	}
	if len(only) > 0 {
		wanted := make(map[string]struct{}, len(only))
		for _, name := range only {
			wanted[name] = struct{}{}
		}
		filtered := locals[:0]
		for _, local := range locals {
			if _, ok := wanted[local.Name]; ok {
				filtered = append(filtered, local)
			}
		}
		locals = filtered
	}

	remotes, err := deps.listRemotes(root)
	if err != nil {
		return output.Report{}, fmt.Errorf("listing remote branches: %w", err)
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

	pending := make([]struct {
		local        git.BranchInfo
		info         classify.BranchInfo
		remoteExists bool
	}, 0, len(locals))
	prNeed := make([]string, 0, len(locals))

	for _, local := range locals {
		_, remoteExists := remotes[local.Name]
		excluded := excludedName(local.Name, classCfg)
		skipMerge := local.IsCurrent || inWorktree[local.Name] || stashed[local.Name] || remoteExists || excluded
		info, err := classifyInfo(root, local, defaultBranch, inWorktree[local.Name], stashed[local.Name], skipMerge)
		if err != nil {
			return output.Report{}, err
		}
		if !remoteExists && !skipMerge && !info.IsAncestor {
			prNeed = append(prNeed, local.Name)
		}
		pending = append(pending, struct {
			local        git.BranchInfo
			info         classify.BranchInfo
			remoteExists bool
		}{local: local, info: info, remoteExists: remoteExists})
	}

	prs, prsChecked, err := lookupPRs(ctx, deps, repo, prNeed, warnings)
	if err != nil {
		return output.Report{}, err
	}

	results := make([]classify.BranchResult, 0, len(pending))
	for _, item := range pending {
		pr := classify.PRStatus{}
		if match, ok := prs[item.local.Name]; ok {
			pr = classify.PRStatus{Found: match.Found, Merged: match.Merged, Number: match.Number, MergedAt: match.MergedAt}
		}
		results = append(results, classify.Classify(item.info, classify.RemoteStatus{Exists: item.remoteExists}, pr, defaultBranch, classCfg))
	}

	return output.Report{
		DefaultBranch: defaultBranch,
		LocalOnly:     false,
		PRsChecked:    prsChecked,
		Results:       results,
	}, nil
}

func resolveDefaultBranch(ctx context.Context, root string, repo ghub.Repo, deps *scanDeps, warnings io.Writer, override string) (string, error) {
	if override != "" {
		exists, err := git.LocalBranchExists(root, override)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("default_branch %q from config does not exist locally", override)
		}
		return override, nil
	}

	name, err := git.GetDefaultBranch(root)
	if err == nil {
		return name, nil
	}
	if !errors.Is(err, git.ErrDefaultBranchUnknown) {
		return "", err
	}

	token, tokErr := deps.store.Get()
	if tokErr != nil {
		return "", fmt.Errorf("cannot determine the default branch: %w (run `git remote set-head origin -a` or `deadwood auth login`)", err)
	}
	name, ghErr := deps.ghDefault(ctx, token.Value, repo)
	if ghErr != nil {
		return "", fmt.Errorf("cannot determine the default branch: %w", ghErr)
	}
	if token.FromEnv && warnings != nil {
		fmt.Fprintf(warnings, "warning: using %s; this is less secure than the OS keychain\n", auth.EnvToken)
	}
	return name, nil
}

func lookupPRs(ctx context.Context, deps *scanDeps, repo ghub.Repo, branches []string, warnings io.Writer) (map[string]ghub.PRMatch, bool, error) {
	if len(branches) == 0 {
		return nil, true, nil
	}
	token, err := deps.store.Get()
	if errors.Is(err, auth.ErrNotFound) {
		if warnings != nil {
			fmt.Fprintln(warnings, "warning: not logged in; squash-merged detection skipped. Run `deadwood auth login`.")
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if token.FromEnv && warnings != nil {
		fmt.Fprintf(warnings, "warning: using %s; this is less secure than the OS keychain\n", auth.EnvToken)
	}
	matches, err := deps.mergedPRs(ctx, token.Value, repo, branches)
	if err != nil {
		return nil, false, err
	}
	return matches, true, nil
}

func excludedName(name string, cfg classify.Config) bool {
	for _, pattern := range cfg.ExcludePatterns {
		ok, err := path.Match(pattern, name)
		if err == nil && ok {
			return true
		}
	}
	return false
}

func classifyInfo(repoPath string, local git.BranchInfo, defaultBranch string, hasWorktree, hasStash, skipMerge bool) (classify.BranchInfo, error) {
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

	if skipMerge {
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
