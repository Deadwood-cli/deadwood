// Package output prints scan results. It has no git or GitHub dependency: the
// cli layer hands it already-classified BranchResults.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/Deadwood-cli/deadwood/internal/classify"
)

// Options control which form of the report is written.
type Options struct {
	Verbose bool
	JSON    bool
}

// Report is one scan's worth of classified branches.
type Report struct {
	DefaultBranch string
	// LocalOnly is true when remotes were not consulted.
	LocalOnly bool
	// PRsChecked is true when squash-merge PR matching ran.
	PRsChecked bool
	Results    []classify.BranchResult
}

type bucketMeta struct {
	emoji string
	label string
}

var bucketOrder = []classify.Bucket{
	classify.BucketSafeDelete,
	classify.BucketSquashMerged,
	classify.BucketNeedsReview,
	classify.BucketActive,
	classify.BucketProtected,
}

var bucketMetaBy = map[classify.Bucket]bucketMeta{
	classify.BucketSafeDelete:   {emoji: "✓", label: "Safe to delete"},
	classify.BucketSquashMerged: {emoji: "◆", label: "Squash-merged"},
	classify.BucketNeedsReview:  {emoji: "▲", label: "Needs review"},
	classify.BucketActive:       {emoji: "●", label: "Active (remote live)"},
	classify.BucketProtected:    {emoji: "■", label: "Protected"},
}

// Write prints the report to w. JSON is a complete dump of every branch;
// the human form lists per-branch detail only when verbose.
func Write(w io.Writer, report Report, opts Options) error {
	if opts.JSON {
		return writeJSON(w, report)
	}
	return writeHuman(w, report, opts.Verbose)
}

func writeHuman(w io.Writer, report Report, verbose bool) error {
	counts := countByBucket(report.Results)
	header := fmt.Sprintf("Deadwood scan — %d local branches", len(report.Results))
	if report.LocalOnly {
		header += " (local-only)"
	}
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	for _, bucket := range bucketOrder {
		meta := bucketMetaBy[bucket]
		if _, err := fmt.Fprintf(w, "    %s %-22s %d\n", meta.emoji, meta.label, counts[bucket]); err != nil {
			return err
		}
	}

	if verbose {
		if err := writeVerbose(w, report.Results); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if report.LocalOnly {
		if _, err := fmt.Fprintln(w, "Remote branches and pull requests were not checked."); err != nil {
			return err
		}
	} else if !report.PRsChecked {
		if _, err := fmt.Fprintln(w, "Squash-merged detection skipped; run `deadwood auth login`."); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "Run `deadwood clean` to review and delete.")
	return err
}

func writeVerbose(w io.Writer, results []classify.BranchResult) error {
	grouped := groupByBucket(results)
	for _, bucket := range bucketOrder {
		members := grouped[bucket]
		if len(members) == 0 {
			continue
		}
		meta := bucketMetaBy[bucket]
		if _, err := fmt.Fprintf(w, "\n%s %s\n", meta.emoji, meta.label); err != nil {
			return err
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, result := range members {
			if _, err := fmt.Fprintf(tw, "  %s\t%s\t%s\n", result.Branch.Name, result.Confidence, result.Reason); err != nil {
				return err
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

type jsonReport struct {
	DefaultBranch string         `json:"default_branch"`
	LocalOnly     bool           `json:"local_only"`
	PRsChecked    bool           `json:"prs_checked"`
	BranchCount   int            `json:"branch_count"`
	Counts        map[string]int `json:"counts"`
	Branches      []jsonBranch   `json:"branches"`
}

type jsonBranch struct {
	Name       string `json:"name"`
	Bucket     string `json:"bucket"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
}

func writeJSON(w io.Writer, report Report) error {
	counts := countByBucket(report.Results)
	payload := jsonReport{
		DefaultBranch: report.DefaultBranch,
		LocalOnly:     report.LocalOnly,
		PRsChecked:    report.PRsChecked,
		BranchCount:   len(report.Results),
		Counts:        make(map[string]int, len(bucketOrder)),
		Branches:      make([]jsonBranch, 0, len(report.Results)),
	}
	for _, bucket := range bucketOrder {
		payload.Counts[string(bucket)] = counts[bucket]
	}
	for _, result := range report.Results {
		payload.Branches = append(payload.Branches, jsonBranch{
			Name:       result.Branch.Name,
			Bucket:     string(result.Bucket),
			Reason:     result.Reason,
			Confidence: result.Confidence,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func countByBucket(results []classify.BranchResult) map[classify.Bucket]int {
	counts := make(map[classify.Bucket]int, len(bucketOrder))
	for _, result := range results {
		counts[result.Bucket]++
	}
	return counts
}

func groupByBucket(results []classify.BranchResult) map[classify.Bucket][]classify.BranchResult {
	grouped := make(map[classify.Bucket][]classify.BranchResult, len(bucketOrder))
	for _, result := range results {
		grouped[result.Bucket] = append(grouped[result.Bucket], result)
	}
	return grouped
}
