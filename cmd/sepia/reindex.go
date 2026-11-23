package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/AutumnsGrove/sepia/internal/config"
	"github.com/AutumnsGrove/sepia/internal/store"
	"github.com/AutumnsGrove/sepia/internal/version"
)

// reindex rebuilds the library's search index from the entries and works.
//
// Every search already notices when the index holds a different number of entries from the library and rebuilds it,
// so this command is not needed for the ordinary case. It exists for the case the counts cannot see: a file restored
// from an older backup whose index rows are the right number and the wrong text, or a hand edit to a body. It is safe
// to run at any time, because the index is derived data and no review is read or written.
func reindex(ctx context.Context, cfg config.Config, stdout, stderr io.Writer) int {
	db, err := store.Open(ctx, store.Options{Path: filepath.Join(cfg.DataDir, "sepia.db")})
	if err != nil {
		fmt.Fprintf(stderr, "sepia: reindex: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()
	if err = db.Migrate(ctx); err != nil {
		fmt.Fprintf(stderr, "sepia: reindex: %v\n", err)
		return 1
	}
	written, err := db.RebuildSearchIndex(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "sepia: reindex: %v\n", err)
		return 1
	}
	noun := "entries"
	if written == 1 {
		noun = "entry"
	}
	fmt.Fprintf(stdout, "search index rebuilt: %d %s, %s\n", written, noun, version.Version)
	return 0
}
