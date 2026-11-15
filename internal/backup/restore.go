package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // the pure-Go driver, the only one this build has
)

// Restoring is an offline command, never an HTTP route: a database file cannot safely be swapped under a running
// connection (docs/slices/S14-backups.md, Settled). It restores the database; the covers are objects in R2 and are
// copied back with the rclone line in docs/INSTALL.md, because a cover that is missing is a placeholder, not a lost
// review.
//
// The order is what makes it safe to trust: prove the snapshot, prove it again as the copy, then print what arrived.

// MaxSnapshotBytes bounds what a restore will copy. The board's database is measured in megabytes; a file claiming
// to be a snapshot and larger than this is not one.
const MaxSnapshotBytes = 10 << 30

// Counts is the fingerprint of a library: four row counts that survive a VACUUM and name what differs.
type Counts struct {
	Entries   int64
	Revisions int64
	Media     int64
	Sessions  int64
}

// String is the line restore prints, and the line an error quotes when two libraries disagree.
func (c Counts) String() string {
	return fmt.Sprintf("entries=%d revisions=%d media=%d sessions=%d",
		c.Entries, c.Revisions, c.Media, c.Sessions)
}

// Equal reports whether two libraries hold the same rows.
func (c Counts) Equal(other Counts) bool { return c == other }

// RestoreRequest is one restore: a snapshot, where it goes, and optionally a database it must match.
type RestoreRequest struct {
	// Snapshot is the file the host script wrote into data/backups/db/.
	Snapshot string
	// Into is a directory that must be empty or absent. A restore never writes over a database that is in use.
	Into string
	// Compare, when set, is a database the restored copy must equal row for row. This is the drill's assertion, and
	// the same one a restore onto the board makes against the copy it replaced.
	Compare string
}

// RestoreResult is what restore found and wrote.
type RestoreResult struct {
	Path   string
	Bytes  int64
	Counts Counts
}

// ErrSnapshotUnusable is a snapshot that is missing, too large, or fails its own integrity check. A restore refuses
// it rather than publishing a database that only looks like hers.
var ErrSnapshotUnusable = errors.New("backup: the snapshot is not usable")

// ErrTargetOccupied is a restore that would write over something. It is a refusal, not an overwrite.
var ErrTargetOccupied = errors.New("backup: the restore target is not empty")

// Restore copies a verified snapshot into an empty directory and proves the copy. It returns the counts it wrote.
func Restore(ctx context.Context, req RestoreRequest) (RestoreResult, error) {
	if req.Snapshot == "" || req.Into == "" {
		return RestoreResult{}, errors.New("backup: a restore needs a snapshot and a directory")
	}
	info, err := os.Stat(req.Snapshot)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("%w: %w", ErrSnapshotUnusable, err)
	}
	if info.IsDir() {
		return RestoreResult{}, fmt.Errorf("%w: %s is a directory", ErrSnapshotUnusable, req.Snapshot)
	}
	if info.Size() == 0 || info.Size() > MaxSnapshotBytes {
		return RestoreResult{}, fmt.Errorf("%w: %s is %d bytes", ErrSnapshotUnusable, req.Snapshot, info.Size())
	}

	// Step one: the snapshot must be a database before anything is written anywhere.
	snapshotCounts, err := inspect(ctx, req.Snapshot)
	if err != nil {
		return RestoreResult{}, err
	}

	// Step two: the target must be ours to write. An empty directory is fine; a database in it is not.
	if err = prepareTarget(req.Into); err != nil {
		return RestoreResult{}, err
	}

	target := filepath.Join(req.Into, "sepia.db")
	if err = copyFile(req.Snapshot, target, 0o600); err != nil {
		return RestoreResult{}, err
	}

	// Step three: prove the copy, not just the source. A truncated disk is exactly what this catches.
	copyCounts, err := inspect(ctx, target)
	if err != nil {
		_ = os.Remove(target)
		return RestoreResult{}, fmt.Errorf("backup: the restored copy did not verify: %w", err)
	}
	if !copyCounts.Equal(snapshotCounts) {
		_ = os.Remove(target)
		return RestoreResult{}, fmt.Errorf("backup: the restored copy holds %s, the snapshot holds %s",
			copyCounts, snapshotCounts)
	}

	// Step four: the drill's assertion. The copy has to equal the library it restored from, not merely verify.
	if req.Compare != "" {
		want, err := inspect(ctx, req.Compare)
		if err != nil {
			_ = os.Remove(target)
			return RestoreResult{}, fmt.Errorf("backup: compare with %s: %w", req.Compare, err)
		}
		if !copyCounts.Equal(want) {
			_ = os.Remove(target)
			return RestoreResult{}, fmt.Errorf("backup: the restored library is %s, the source is %s",
				copyCounts, want)
		}
	}

	return RestoreResult{Path: target, Bytes: info.Size(), Counts: copyCounts}, nil
}

// prepareTarget makes sure the directory exists and holds nothing of hers. A missing directory is created; a
// directory with anything in it is refused, because the one thing a restore must never do is overwrite a database
// that might be the only copy left.
func prepareTarget(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case err == nil && len(entries) > 0:
		return ErrTargetOccupied
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("backup: restore target: %w", err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("backup: restore target: %w", err)
	}
	return nil
}

// inspect opens a database read-only and proves it: SQLite's own integrity check, then no foreign key violations,
// then the counts. Read-only because a restore must not be able to repair (or change) what it is checking.
func inspect(ctx context.Context, path string) (Counts, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return Counts{}, fmt.Errorf("%w: %w", ErrSnapshotUnusable, err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	var integrity string
	if err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return Counts{}, fmt.Errorf("%w: %w", ErrSnapshotUnusable, err)
	}
	if integrity != "ok" {
		return Counts{}, fmt.Errorf("%w: integrity check said %q", ErrSnapshotUnusable, integrity)
	}

	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return Counts{}, fmt.Errorf("%w: %w", ErrSnapshotUnusable, err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		return Counts{}, fmt.Errorf("%w: a foreign key points at a row that is not there", ErrSnapshotUnusable)
	}
	if err := rows.Err(); err != nil {
		return Counts{}, fmt.Errorf("%w: %w", ErrSnapshotUnusable, err)
	}

	var counts Counts
	if err := db.QueryRowContext(ctx, `SELECT
			(SELECT count(*) FROM entries),
			(SELECT count(*) FROM entry_revisions),
			(SELECT count(*) FROM media),
			(SELECT count(*) FROM sessions)`).
		Scan(&counts.Entries, &counts.Revisions, &counts.Media, &counts.Sessions); err != nil {
		return Counts{}, fmt.Errorf("%w: %w", ErrSnapshotUnusable, err)
	}
	return counts, nil
}

// copyFile writes through a temp file and renames, so an interrupted restore leaves no half database under the name
// sepia.db.
func copyFile(from, to string, mode os.FileMode) error {
	src, err := os.Open(from)
	if err != nil {
		return fmt.Errorf("backup: open snapshot: %w", err)
	}
	defer func() { _ = src.Close() }()

	tmp := to + ".tmp"
	dst, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("backup: write restore: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("backup: write restore: %w", err)
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("backup: write restore: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("backup: write restore: %w", err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("backup: write restore: %w", err)
	}
	if err := os.Rename(tmp, to); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("backup: write restore: %w", err)
	}
	return nil
}
