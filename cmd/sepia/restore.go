package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/AutumnsGrove/sepia/internal/backup"
)

// restoreArgs is the whole flag set: a snapshot, a directory, and an optional database to compare against. There is
// no default for either path, because a restore that guesses where to write is a restore that can overwrite the only
// copy left.
func restoreArgs(args []string) (backup.RestoreRequest, error) {
	var req backup.RestoreRequest
	for i := 0; i < len(args); i++ {
		value := func(name string) (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a path", name)
			}
			i++
			return args[i], nil
		}
		var err error
		switch {
		case args[i] == "--snapshot":
			req.Snapshot, err = value("--snapshot")
		case strings.HasPrefix(args[i], "--snapshot="):
			req.Snapshot = strings.TrimPrefix(args[i], "--snapshot=")
		case args[i] == "--into":
			req.Into, err = value("--into")
		case strings.HasPrefix(args[i], "--into="):
			req.Into = strings.TrimPrefix(args[i], "--into=")
		case args[i] == "--compare":
			req.Compare, err = value("--compare")
		case strings.HasPrefix(args[i], "--compare="):
			req.Compare = strings.TrimPrefix(args[i], "--compare=")
		default:
			return backup.RestoreRequest{}, fmt.Errorf("unknown argument %q", args[i])
		}
		if err != nil {
			return backup.RestoreRequest{}, err
		}
	}
	if strings.TrimSpace(req.Snapshot) == "" {
		return backup.RestoreRequest{}, fmt.Errorf("--snapshot is required")
	}
	if strings.TrimSpace(req.Into) == "" {
		return backup.RestoreRequest{}, fmt.Errorf("--into is required")
	}
	return req, nil
}

// runRestore performs one restore and prints what arrived. It is the offline half of the backup flow: the snapshot
// and the mirror are the host's job, and this is how a snapshot becomes a database again.
func runRestore(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	req, err := restoreArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "sepia: restore: %v\n\n%s", err, usage)
		return 2
	}
	result, err := backup.Restore(ctx, req)
	if err != nil {
		fmt.Fprintf(stderr, "sepia: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "restored %s into %s\n%s\n", humanBytes(result.Bytes), result.Path, result.Counts)
	return 0
}

// humanBytes is the short size the command prints. The board's backups are megabytes, so decimal units are close
// enough and the number never claims more precision than it has.
func humanBytes(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1f GB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0f kB", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
