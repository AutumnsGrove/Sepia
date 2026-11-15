package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// fakeStore is the mirrors table without SQLite: a map keyed by id, so a second import of the same receipt is
// visibly a no-op rather than a duplicate row.
type fakeStore struct {
	rows map[string]domain.Mirror
	// fail makes RecordMirror return an error, for the route where the database is the thing that broke.
	fail error
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]domain.Mirror{}} }

func (f *fakeStore) RecordMirror(_ context.Context, m domain.Mirror) error {
	if f.fail != nil {
		return f.fail
	}
	if _, ok := f.rows[m.ID]; ok {
		return nil
	}
	f.rows[m.ID] = m
	return nil
}

func (f *fakeStore) Mirrors(_ context.Context, limit int) ([]domain.Mirror, error) {
	out := make([]domain.Mirror, 0, len(f.rows))
	for _, m := range f.rows {
		out = append(out, m)
	}
	// Newest first, the order the query returns.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].RanAt.After(out[i].RanAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func line(run, target, status string, at time.Time, bytes int64, detail string) string {
	return fmt.Sprintf(`{"run":%q,"ran_at":%q,"target":%q,"status":%q,"bytes":%d,"detail":%q}`,
		run, at.Format(time.RFC3339), target, status, bytes, detail)
}

// TestParseKeepsWhatItCanTrust is the bad route of the import: a corrupt line is skipped and counted, and every
// usable line after it still arrives.
func TestParseKeepsWhatItCanTrust(t *testing.T) {
	at := time.Date(2026, 11, 15, 3, 30, 0, 0, time.UTC)
	input := strings.Join([]string{
		line("run-1", "r2sepia:sepia-backups/db", "ok", at, 4200000, ""),
		"",
		"{not json",
		line("run-1", "r2sepia:sepia-backups/covers", "failed", at, 0, "AccessDenied"),
		`{"run":"run-1","ran_at":"2026-11-15T03:30:00Z","target":"r2sepia:sepia-backups/db","status":"ok","bytes":1}`,
		line("run-2", "r2sepia:sepia-backups/db", "exploded", at, 1, ""),
		line("", "r2sepia:sepia-backups/db", "ok", at, 1, ""),
		line("run-3", "r2sepia:sepia-backups/db", "ok", at, -5, ""),
		line("run-4", "r2sepia:sepia-backups/db", "ok", at, 1, strings.Repeat("x", 3000)),
		line("run-5", "r2sepia:sepia-backups/db", "ok", at, 1, "line\nbreak\x07bell"),
		"",
	}, "\n")

	got := Parse(strings.NewReader(input))

	if len(got.Items) != 3 {
		t.Fatalf("items = %d, want 3: %+v (skipped %d)", len(got.Items), got.Items, got.Skipped)
	}
	if got.Skipped != 5 {
		t.Errorf("skipped = %d, want 5", got.Skipped)
	}
	if got.Items[0].ID != "run-1:r2sepia:sepia-backups/db" {
		t.Errorf("first id = %q", got.Items[0].ID)
	}
	if got.Items[0].Status != domain.MirrorOK || got.Items[0].Bytes != 4200000 {
		t.Errorf("first receipt = %+v", got.Items[0])
	}
	if got.Items[1].Status != domain.MirrorFailed || got.Items[1].Detail != "AccessDenied" {
		t.Errorf("second receipt = %+v", got.Items[1])
	}
	if len(got.Items[2].Detail) != MaxDetail {
		t.Errorf("detail length = %d, want it cut to %d", len(got.Items[2].Detail), MaxDetail)
	}
	if strings.ContainsAny(got.Items[2].Detail, "\n\r") {
		t.Errorf("detail kept a newline: %q", got.Items[2].Detail)
	}
}

// TestParseDropsTheDuplicate: the id is the run plus the target, so the same record twice is one receipt.
func TestParseDropsTheDuplicate(t *testing.T) {
	at := time.Date(2026, 11, 15, 3, 30, 0, 0, time.UTC)
	once := line("run-1", "r2sepia:sepia-backups/db", "ok", at, 1, "")
	got := Parse(strings.NewReader(once + "\n" + once + "\n"))
	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(got.Items))
	}
}

// TestParseBoundsTheRead: a file larger than one read yields at most one read's worth, newest last.
func TestParseBoundsTheRead(t *testing.T) {
	at := time.Date(2026, 11, 15, 3, 30, 0, 0, time.UTC)
	var b strings.Builder
	for i := 0; i < MaxReceipts+50; i++ {
		b.WriteString(line(fmt.Sprintf("run-%03d", i), "r2sepia:sepia-backups/db", "ok", at, 1, ""))
		b.WriteString("\n")
	}
	got := Parse(strings.NewReader(b.String()))
	if len(got.Items) != MaxReceipts {
		t.Fatalf("items = %d, want %d", len(got.Items), MaxReceipts)
	}
	if got.Items[len(got.Items)-1].ID != fmt.Sprintf("run-%03d:r2sepia:sepia-backups/db", MaxReceipts+49) {
		t.Errorf("the newest receipt was the one dropped: last = %q", got.Items[len(got.Items)-1].ID)
	}
}

// TestViewImportsOnceAndReportsTheLastRun is the happy route: three runs on disk become three receipts, the newest
// is the last, and a second look writes nothing new.
func TestViewImportsOnceAndReportsTheLastRun(t *testing.T) {
	now := time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC)
	tree := newTree(t)
	tree.receipts(t, []string{
		line("run-1", "r2sepia:sepia-backups/db", "ok", now.Add(-24*time.Hour), 4_000_000, ""),
		line("run-1", "r2sepia:sepia-backups/covers", "skipped", now.Add(-24*time.Hour), 0, "nothing to mirror"),
		line("run-2", "r2sepia:sepia-backups/db", "ok", now.Add(-time.Hour), 4_200_000, ""),
	})
	tree.heartbeat(t, now.Add(-time.Hour))
	store := newFakeStore()
	service := New(tree.data, tree.signal, store, func() time.Time { return now })

	view, err := service.View(context.Background())
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.Runs) != 3 {
		t.Fatalf("runs = %d, want 3: %+v", len(view.Runs), view.Runs)
	}
	if view.Last == nil || view.Last.ID != "run-2:r2sepia:sepia-backups/db" {
		t.Errorf("last = %+v, want the newest receipt", view.Last)
	}
	if view.Last.Bytes != 4_200_000 {
		t.Errorf("last bytes = %d", view.Last.Bytes)
	}
	if view.Helper.State != HelperReady || view.Helper.HeartbeatAt == nil {
		t.Errorf("helper = %+v, want ready with a heartbeat", view.Helper)
	}
	if view.Pending {
		t.Error("pending is true with no request")
	}

	if _, err := service.Import(context.Background()); err != nil {
		t.Fatalf("second import: %v", err)
	}
	if len(store.rows) != 3 {
		t.Errorf("rows = %d after a second import, want 3", len(store.rows))
	}
}

// TestViewOnABareDeployment: no heartbeat and no receipts is a normal state, not an error.
func TestViewOnABareDeployment(t *testing.T) {
	now := time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC)
	tree := newTree(t)
	service := New(tree.data, "", newFakeStore(), func() time.Time { return now })

	view, err := service.View(context.Background())
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Helper.State != HelperNotInstalled {
		t.Errorf("helper = %q, want %q", view.Helper.State, HelperNotInstalled)
	}
	if view.Last != nil || len(view.Runs) != 0 {
		t.Errorf("runs = %+v, want none", view.Runs)
	}
	if err := service.Request(); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("request err = %v, want ErrNotInstalled", err)
	}
}

// TestHelperGoesStale: a timer that stopped is not the same as one that never ran.
func TestHelperGoesStale(t *testing.T) {
	now := time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC)
	tree := newTree(t)
	tree.heartbeat(t, now.Add(-StaleAfter-time.Minute))
	service := New(tree.data, tree.signal, newFakeStore(), func() time.Time { return now })

	view, err := service.View(context.Background())
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Helper.State != HelperStale {
		t.Errorf("helper = %q, want %q", view.Helper.State, HelperStale)
	}
}

// TestRequestWritesOnceWhileWaiting: the second tap joins the request already waiting instead of queueing another.
func TestRequestWritesOnceWhileWaiting(t *testing.T) {
	now := time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC)
	tree := newTree(t)
	service := New(tree.data, tree.signal, newFakeStore(), func() time.Time { return now })

	if err := service.Request(); err != nil {
		t.Fatalf("request: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(tree.signal, requestName))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	if string(body) != "action=backup\n" {
		t.Errorf("request body = %q", body)
	}
	if secondErr := service.Request(); !errors.Is(secondErr, ErrPending) {
		t.Errorf("second request err = %v, want ErrPending", secondErr)
	}
	entries, err := os.ReadDir(tree.signal)
	if err != nil {
		t.Fatalf("read signal dir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp.") {
			t.Errorf("a temp file was left behind: %s", entry.Name())
		}
	}
}

// TestRequestReplacesARequestNobodyServed: after a few minutes the old file is a leftover, not a pending run.
func TestRequestReplacesARequestNobodyServed(t *testing.T) {
	now := time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC)
	tree := newTree(t)
	old := filepath.Join(tree.signal, requestName)
	if err := os.WriteFile(old, []byte("action=backup\n"), 0o644); err != nil {
		t.Fatalf("write old request: %v", err)
	}
	stale := now.Add(-PendingFor - time.Minute)
	if err := os.Chtimes(old, stale, stale); err != nil {
		t.Fatalf("age the request: %v", err)
	}
	service := New(tree.data, tree.signal, newFakeStore(), func() time.Time { return now })

	if err := service.Request(); err != nil {
		t.Fatalf("request over a leftover: %v", err)
	}
	info, err := os.Stat(old)
	if err != nil {
		t.Fatalf("stat request: %v", err)
	}
	if info.ModTime().Before(stale) {
		t.Error("the leftover request was not replaced")
	}
}

// TestViewReadsTheNewestEndOfALargeFile is the bound on a file that grew past one read: the oldest runs fall out of
// the window and the newest are still imported.
func TestViewReadsTheNewestEndOfALargeFile(t *testing.T) {
	now := time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC)
	tree := newTree(t)
	at := now.Add(-2 * time.Hour)
	var lines []string
	for i := 0; i < 4000; i++ {
		lines = append(lines, line(fmt.Sprintf("run-%04d", i), "r2sepia:sepia-backups/db", "ok", at, 1, ""))
	}
	tree.receipts(t, lines)
	store := newFakeStore()
	service := New(tree.data, tree.signal, store, func() time.Time { return now })

	view, err := service.View(context.Background())
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(store.rows) == 0 {
		t.Fatal("nothing was imported from a large file")
	}
	if _, err := os.Stat(filepath.Join(tree.data, backupsDir, receiptsName)); err != nil {
		t.Fatalf("stat receipts: %v", err)
	}
	// The newest run must be among them, which is only true if the read started at the end.
	newest := "run-3999:r2sepia:sepia-backups/db"
	if _, ok := store.rows[newest]; !ok {
		t.Errorf("the newest receipt was not imported; %d rows came from the wrong end of the file", len(store.rows))
	}
	if view.Last == nil {
		t.Error("view has no last run")
	}
}

// TestImportFailureIsReported: a database error is not swallowed, and the screen can say so.
func TestImportFailureIsReported(t *testing.T) {
	now := time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC)
	tree := newTree(t)
	tree.receipts(t, []string{line("run-1", "r2sepia:sepia-backups/db", "ok", now, 1, "")})
	store := newFakeStore()
	store.fail = errors.New("database is locked")
	service := New(tree.data, tree.signal, store, func() time.Time { return now })

	if _, err := service.View(context.Background()); err == nil {
		t.Fatal("a failed import did not surface")
	}
}

// tree is a data directory shaped like the board's, plus a signal directory.
type tree struct {
	data   string
	signal string
}

func newTree(t *testing.T) tree {
	t.Helper()
	root := t.TempDir()
	tr := tree{data: filepath.Join(root, "data"), signal: filepath.Join(root, "update-signal")}
	for _, dir := range []string{filepath.Join(tr.data, backupsDir), tr.signal} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	return tr
}

func (tr tree) receipts(t *testing.T, lines []string) {
	t.Helper()
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tr.data, backupsDir, receiptsName), []byte(body), 0o600); err != nil {
		t.Fatalf("write receipts: %v", err)
	}
}

func (tr tree) heartbeat(t *testing.T, at time.Time) {
	t.Helper()
	path := filepath.Join(tr.data, backupsDir, heartbeatName)
	if err := os.WriteFile(path, []byte(at.Format(time.RFC3339)+"\n"), 0o600); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatalf("age heartbeat: %v", err)
	}
}

// FuzzParse asserts the parser returns, returns in bounded memory and never yields something the store would refuse.
func FuzzParse(f *testing.F) {
	at := time.Date(2026, 11, 15, 3, 30, 0, 0, time.UTC)
	f.Add(line("run-1", "r2sepia:sepia-backups/db", "ok", at, 4200000, ""))
	f.Add(line("run-1", "r2sepia:sepia-backups/covers", "failed", at, 0, "AccessDenied"))
	f.Add("{")
	f.Add("")
	f.Add(`{"run":"a","ran_at":"2026-11-15T03:30:00Z","target":"t","status":"ok","bytes":0,"detail":""}`)

	f.Fuzz(func(t *testing.T, input string) {
		parsed := Parse(strings.NewReader(input))
		if len(parsed.Items) > MaxReceipts {
			t.Fatalf("Parse returned %d receipts, over the %d bound", len(parsed.Items), MaxReceipts)
		}
		if parsed.Skipped < 0 {
			t.Fatalf("negative skipped count")
		}
		for _, item := range parsed.Items {
			if !item.Status.Valid() {
				t.Fatalf("status %q would violate the database's CHECK", item.Status)
			}
			if item.ID == "" || len([]rune(item.Target)) > MaxTarget || len([]rune(item.Detail)) > MaxDetail {
				t.Fatalf("receipt out of bounds: %+v", item)
			}
			if strings.ContainsAny(item.Target, "\n\r") || strings.ContainsAny(item.Detail, "\n\r") {
				t.Fatalf("receipt carries a newline: %+v", item)
			}
		}
	})
}
