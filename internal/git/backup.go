package git

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// BackupRefPrefix namespaces deadwood's backup refs. Living outside refs/heads
// keeps them off `git branch` listings while still pinning the commits against
// garbage collection.
const BackupRefPrefix = "refs/deadwood-backup/"

var (
	// ErrBackupMissing reports that no backup ref exists for a branch. Deleting
	// without one is never permitted.
	ErrBackupMissing = errors.New("no backup ref exists for this branch")

	// ErrBackupStale reports that a backup ref exists but records a different
	// commit than the branch currently points at, so restoring from it would
	// silently lose the newer commits.
	ErrBackupStale = errors.New("backup ref does not point at the branch's current tip")
)

func backupRefName(branch string) string { return BackupRefPrefix + branch }

// BackupInfo is one refs/deadwood-backup entry, for `deadwood undo --list`.
type BackupInfo struct {
	Branch     string
	SHA        string
	CommitDate time.Time
	Subject    string
}

var backupFormat = strings.Join([]string{
	"%(refname)",
	"%(objectname)",
	"%(committerdate:iso-strict)",
	"%(contents:subject)",
}, fieldSepForEachRef)

const backupFieldCount = 4

// CreateBackupRef records a branch's current tip under refs/deadwood-backup/
// so the branch can be recreated after deletion.
//
// This must succeed before the branch is deleted; DeleteBranch independently
// re-checks the result rather than trusting the caller's sequencing. The ref is
// read back after writing, because a backup that only appears to exist is worse
// than none at all.
//
// Creating a backup for "a" fails while a backup for "a/b" exists, since git
// cannot store a ref file and a ref directory under the same name. That is
// reported to the caller, which skips the branch.
func CreateBackupRef(repoPath, branch string) error {
	if err := validateRefArgument(branch); err != nil {
		return err
	}

	tip, err := resolveRef(repoPath, "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("reading tip of branch %q: %w", branch, err)
	}

	ref := backupRefName(branch)
	if _, err := runMutating(repoPath, "update-ref", "-m", "deadwood backup", ref, tip); err != nil {
		return fmt.Errorf("creating backup ref %s: %w", ref, err)
	}

	stored, err := resolveRef(repoPath, ref)
	if err != nil {
		return fmt.Errorf("verifying backup ref %s: %w", ref, err)
	}
	if stored != tip {
		return fmt.Errorf("backup ref %s records %s but branch %q is at %s", ref, stored, branch, tip)
	}
	return nil
}

// ListBackupRefs returns the branch names that have a backup ref, sorted.
func ListBackupRefs(repoPath string) ([]string, error) {
	details, err := ListBackupDetails(repoPath)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(details))
	for i, info := range details {
		names[i] = info.Branch
	}
	return names, nil
}

// ListBackupDetails returns backup refs with the commit they pin, sorted by name.
func ListBackupDetails(repoPath string) ([]BackupInfo, error) {
	out, err := run(repoPath, "for-each-ref", "--sort=refname", "--format="+backupFormat, BackupRefPrefix)
	if err != nil {
		return nil, err
	}

	records := lines(out)
	infos := make([]BackupInfo, 0, len(records))
	for _, record := range records {
		info, err := parseBackupRecord(record)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func parseBackupRecord(record string) (BackupInfo, error) {
	fields := strings.Split(record, fieldSep)
	if len(fields) != backupFieldCount {
		return BackupInfo{}, fmt.Errorf("parsing backup record %q: got %d fields, want %d",
			record, len(fields), backupFieldCount)
	}
	committed, err := time.Parse(time.RFC3339, fields[2])
	if err != nil {
		return BackupInfo{}, fmt.Errorf("parsing commit date for backup %q: %w", fields[0], err)
	}
	return BackupInfo{
		Branch:     strings.TrimPrefix(fields[0], BackupRefPrefix),
		SHA:        fields[1],
		CommitDate: committed,
		Subject:    fields[3],
	}, nil
}

// RestoreFromBackup recreates a branch at the commit its backup ref records.
// The backup is left in place; nothing removes it automatically.
func RestoreFromBackup(repoPath, branch string) error {
	if err := validateRefArgument(branch); err != nil {
		return err
	}

	ref := backupRefName(branch)
	exists, err := refExists(repoPath, ref)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("restoring branch %q: %w", branch, ErrBackupMissing)
	}

	// git branch refuses to overwrite an existing branch, which is what we want:
	// restoring must never clobber work that is already there.
	if _, err := runMutating(repoPath, "branch", branch, ref); err != nil {
		return fmt.Errorf("restoring branch %q: %w", branch, err)
	}
	return nil
}

// DeleteBackupRef removes a branch's backup ref.
//
// Nothing in v0.1 calls this. It exists for a future prune command, and the
// retention window in .deadwood.yml is not enforced anywhere.
func DeleteBackupRef(repoPath, branch string) error {
	if err := validateRefArgument(branch); err != nil {
		return err
	}
	if _, err := runMutating(repoPath, "update-ref", "-d", backupRefName(branch)); err != nil {
		return fmt.Errorf("deleting backup ref for %q: %w", branch, err)
	}
	return nil
}

// verifyBackupCoversTip reports an error unless a backup ref exists and records
// exactly the commit the branch currently points at.
func verifyBackupCoversTip(repoPath, branch string) error {
	tip, err := resolveRef(repoPath, "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("reading tip of branch %q: %w", branch, err)
	}

	ref := backupRefName(branch)
	stored, err := run(repoPath, "rev-parse", "--verify", "--quiet", ref)
	if err != nil {
		if code, ok := exitCodeOf(err); ok && code == 1 {
			return fmt.Errorf("branch %q: %w", branch, ErrBackupMissing)
		}
		return fmt.Errorf("reading backup ref %s: %w", ref, err)
	}
	if stored != tip {
		return fmt.Errorf("branch %q is at %s but %s records %s: %w",
			branch, tip, ref, stored, ErrBackupStale)
	}
	return nil
}
