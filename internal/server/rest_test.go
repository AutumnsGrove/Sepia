package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/providers"
	"github.com/AutumnsGrove/sepia/internal/session"
	"github.com/AutumnsGrove/sepia/internal/store"
	"github.com/AutumnsGrove/sepia/internal/world"
)

// The REST half: every route on the happy path, every refusal, and the two sad shapes a page can produce (a gone
// session and a database row that is not there).

func newREST(t *testing.T) (http.Handler, *session.Engine, *store.Store) {
	t.Helper()
	// newSocketRig already has a real store, engine and work; REST shares them.
	rig := newSocketRig(t)
	return rig.server.Config.Handler, rig.engine, rig.db
}

func doJSON(t *testing.T, h http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, target, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "localhost:8893"
	req.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set("Origin", "http://localhost:8893")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSessionsCreateAndList(t *testing.T) {
	h, _, _ := newREST(t)

	rec := doJSON(t, h, http.MethodPost, "/api/sessions", map[string]any{"kind": "entry"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body)
	}
	var created wireSession
	decode(t, rec, &created)
	if created.Kind != "entry" || created.State != "active" || created.WaitingReason == nil ||
		*created.WaitingReason != "pick_work" {
		t.Errorf("session = %+v", created)
	}

	general := doJSON(t, h, http.MethodPost, "/api/sessions", map[string]any{"kind": "general", "title": "next reads"})
	if general.Code != http.StatusCreated {
		t.Fatalf("general = %d: %s", general.Code, general.Body)
	}

	list := doJSON(t, h, http.MethodGet, "/api/sessions?state=active&limit=1", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", list.Code, list.Body)
	}
	var page struct {
		Items      []wireSession `json:"items"`
		NextCursor *string       `json:"next_cursor"`
	}
	decode(t, list, &page)
	if len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("page = %+v", page)
	}
	next := doJSON(t, h, http.MethodGet, "/api/sessions?state=active&limit=1&cursor="+*page.NextCursor, nil)
	var second struct {
		Items []wireSession `json:"items"`
	}
	decode(t, next, &second)
	if len(second.Items) != 1 || second.Items[0].ID == page.Items[0].ID {
		t.Errorf("second page = %+v", second.Items)
	}

	one := doJSON(t, h, http.MethodGet, "/api/sessions/"+created.ID, nil)
	if one.Code != http.StatusOK {
		t.Fatalf("get = %d: %s", one.Code, one.Body)
	}
	if missing := doJSON(t, h, http.MethodGet, "/api/sessions/no-such", nil); missing.Code != http.StatusNotFound {
		t.Errorf("missing session = %d", missing.Code)
	}

	// A general chat has no draft, so no waiting reason.
	var chat wireSession
	decode(t, general, &chat)
	if chat.WaitingReason != nil {
		t.Errorf("a chat has a waiting reason: %v", *chat.WaitingReason)
	}
}

func TestSessionsRefuseBadRequests(t *testing.T) {
	h, _, _ := newREST(t)

	if rec := doJSON(t, h, http.MethodPost, "/api/sessions", map[string]any{"kind": "nonsense"}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown kind = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/sessions", map[string]any{"kind": "entry", "surprise": 1}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/sessions", strings.Repeat("x", maxJSONBody+1)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("huge body = %d", rec.Code)
	}

	// A body sent as a form is refused before it is parsed: a cross-site form can send one.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/sessions",
		strings.NewReader(`{"kind":"entry"}`))
	req.Host = "localhost:8893"
	req.Header.Set("Origin", "http://localhost:8893")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("wrong content type = %d", rec.Code)
	}

	if rec := doJSON(t, h, http.MethodGet, "/api/sessions?state=nonsense", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown state = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/sessions?limit=zero", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad limit = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/sessions?cursor=not!base64", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad cursor = %d", rec.Code)
	}
}

// aConfirmedEntry makes and confirms one saved entry with a finished date, so the library order is deterministic.
func aConfirmedEntry(t *testing.T, engine *session.Engine, db *store.Store, title, finished string) domain.Entry {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveWork(ctx, domain.Work{
		ID: workIDFor(title), Medium: "book", Title: title, Verified: true,
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.StartDraft(ctx, sess.ID, domain.Entry{WorkID: workIDFor(title)}); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.EditDraft(ctx, sess.ID, session.Edit{BaseVersion: 0, Set: map[string]json.RawMessage{
		"rating":      json.RawMessage("18"),
		"body":        json.RawMessage(`"A quiet book about light and memory."`),
		"finished_on": json.RawMessage(`"` + finished + `"`),
	}, ClientMsgID: "e-" + title}); err != nil {
		t.Fatal(err)
	}
	entry, _, _, err := engine.Confirm(ctx, sess.ID, session.ConfirmRequest{BaseVersion: 1, Revises: ""})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

// aReviewedEntry confirms one entry with the body, rating and date a search or filter case needs. It goes through the
// engine's own confirm, so the row is written exactly as the app writes it.
func aReviewedEntry(t *testing.T, engine *session.Engine, db *store.Store, title, medium, body string, rating int,
	finished string,
) domain.Entry {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveWork(ctx, domain.Work{
		ID: workIDFor(title), Medium: medium, Title: title, Verified: true,
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.StartDraft(ctx, sess.ID, domain.Entry{WorkID: workIDFor(title)}); err != nil {
		t.Fatal(err)
	}
	ratingJSON, err := json.Marshal(rating)
	if err != nil {
		t.Fatal(err)
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.EditDraft(ctx, sess.ID, session.Edit{BaseVersion: 0, Set: map[string]json.RawMessage{
		"rating":      ratingJSON,
		"body":        bodyJSON,
		"finished_on": json.RawMessage(`"` + finished + `"`),
	}, ClientMsgID: "e-" + title}); err != nil {
		t.Fatal(err)
	}
	entry, _, _, err := engine.Confirm(ctx, sess.ID, session.ConfirmRequest{BaseVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func workIDFor(title string) string {
	id := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	return "w-" + id
}

func TestLibraryListAndPagination(t *testing.T) {
	h, engine, db := newREST(t)

	empty := doJSON(t, h, http.MethodGet, "/api/entries", nil)
	if empty.Code != http.StatusOK {
		t.Fatalf("empty library = %d: %s", empty.Code, empty.Body)
	}
	var emptyPage struct {
		Items      []wireEntry `json:"items"`
		NextCursor *string     `json:"next_cursor"`
	}
	decode(t, empty, &emptyPage)
	if len(emptyPage.Items) != 0 || emptyPage.NextCursor != nil {
		t.Errorf("empty page = %+v", emptyPage)
	}

	first := aConfirmedEntry(t, engine, db, "The Lantern Keeper", "2026-10-30")
	aConfirmedEntry(t, engine, db, "The Quarry Line", "2026-10-31")

	page := doJSON(t, h, http.MethodGet, "/api/entries?limit=1", nil)
	var got struct {
		Items      []wireEntry `json:"items"`
		NextCursor *string     `json:"next_cursor"`
	}
	decode(t, page, &got)
	if len(got.Items) != 1 || got.Items[0].FinishedOn != "2026-10-31" || got.NextCursor == nil {
		t.Fatalf("page = %+v", got)
	}
	nextPage := doJSON(t, h, http.MethodGet, "/api/entries?limit=1&cursor="+*got.NextCursor, nil)
	var second struct {
		Items      []wireEntry `json:"items"`
		NextCursor *string     `json:"next_cursor"`
	}
	decode(t, nextPage, &second)
	if len(second.Items) != 1 || second.Items[0].FinishedOn != "2026-10-30" || second.NextCursor != nil {
		t.Fatalf("second page = %+v", second)
	}

	one := doJSON(t, h, http.MethodGet, "/api/entries/"+first.ID, nil)
	if one.Code != http.StatusOK {
		t.Fatalf("entry = %d: %s", one.Code, one.Body)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/entries?medium=nonsense", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad medium = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/entries?favourite=maybe", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad favourite = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/entries/no-such", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing entry = %d", rec.Code)
	}
}

func TestEntryRevisionsAndDelete(t *testing.T) {
	h, engine, db := newREST(t)
	entry := aConfirmedEntry(t, engine, db, "The Lantern Keeper", "2026-10-30")

	revs := doJSON(t, h, http.MethodGet, "/api/entries/"+entry.ID+"/revisions", nil)
	if revs.Code != http.StatusOK {
		t.Fatalf("revisions = %d: %s", revs.Code, revs.Body)
	}
	var page struct {
		Items []wireRevision `json:"items"`
	}
	decode(t, revs, &page)
	if len(page.Items) != 0 {
		t.Errorf("a first confirm has revisions: %+v", page.Items)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/entries/no-such/revisions", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing revisions = %d", rec.Code)
	}

	del := doJSON(t, h, http.MethodPost, "/api/entries/"+entry.ID+"/delete", nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", del.Code, del.Body)
	}
	after := doJSON(t, h, http.MethodGet, "/api/entries", nil)
	var listed struct {
		Items []wireEntry `json:"items"`
	}
	decode(t, after, &listed)
	if len(listed.Items) != 0 {
		t.Errorf("a deleted entry is still in the library: %+v", listed.Items)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/entries/no-such/delete", nil); rec.Code != http.StatusNotFound {
		t.Errorf("delete missing = %d", rec.Code)
	}
}

func TestWorksSearchAndTheNotBuiltStubs(t *testing.T) {
	h, _, _ := newREST(t)
	if rec := doJSON(t, h, http.MethodGet, "/api/works/w1", nil); rec.Code != http.StatusOK {
		t.Errorf("work = %d: %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/works/no-such", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing work = %d", rec.Code)
	}

	if rec := doJSON(t, h, http.MethodGet, "/api/search?q=lantern", nil); rec.Code != http.StatusOK {
		t.Errorf("search = %d, want 200", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/search?q=", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("empty search = %d", rec.Code)
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/export"},
	} {
		if rec := doJSON(t, h, route.method, route.path, nil); rec.Code != http.StatusNotImplemented {
			t.Errorf("%s %s = %d, want not built", route.method, route.path, rec.Code)
		}
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/version", nil); rec.Code != http.StatusOK {
		t.Errorf("version = %d: %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/services", nil); rec.Code != http.StatusOK {
		t.Errorf("services = %d: %s", rec.Code, rec.Body)
	}
	// An unknown API path and a wrong method both answer the envelope.
	if rec := doJSON(t, h, http.MethodGet, "/api/nonesuch", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown api path = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/entries", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("wrong method = %d", rec.Code)
	}
}

func TestCoverIsServedWithoutTraversal(t *testing.T) {
	h, _, _ := newREST(t)
	// The rig's data directory is the server's; find it through a known file we write ourselves.
	cfg := testConfig()
	dir := t.TempDir()
	cfg.DataDir = dir
	if err := os.MkdirAll(filepath.Join(dir, "covers"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "covers", "lantern.jpg"), []byte("JPEG-BYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := New(cfg, Deps{}, slog.New(slog.DiscardHandler), nil)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/covers/lantern.jpg", nil)
	req.Host = "localhost:8893"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "JPEG-BYTES" {
		t.Fatalf("cover = %d %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("content type = %q", ct)
	}

	// Every format the cover cache can hold is served with its own content type.
	for name, want := range map[string]string{
		"lantern.png": "image/png", "lantern.webp": "image/webp", "lantern.gif": "image/gif",
		"lantern.unknownext": "application/octet-stream",
	} {
		if err := os.WriteFile(filepath.Join(dir, "covers", name), []byte("BYTES"), 0o600); err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/covers/"+name, nil)
		req.Host = "localhost:8893"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if ct := rec.Header().Get("Content-Type"); ct != want {
			t.Errorf("%s content type = %q, want %q", name, ct, want)
		}
	}

	for _, path := range []string{"/covers/../sepia.db", "/covers/%2e%2e%2fsepia.db", "/covers/nope.jpg"} {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		req.Host = "localhost:8893"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		// A path with ".." is cleaned and redirected by the mux; either way the file is never served.
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMovedPermanently &&
			rec.Code != http.StatusTemporaryRedirect {
			t.Errorf("%s = %d, want a refusal", path, rec.Code)
		}
	}
	_ = h
}

// The media upload is S18. A route that promises a file and stores nothing must say so with a stable code, and it
// must still answer a session that does not exist before it says it.
func TestUploadMediaIsNotBuiltYet(t *testing.T) {
	h, engine, _ := newREST(t)
	ctx := context.Background()
	sess, err := engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, h, http.MethodPost, "/api/sessions/"+sess.ID+"/media", map[string]any{"kind": "screenshot"})
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("upload = %d, want not built: %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/sessions/no-such/media", nil); rec.Code != http.StatusNotFound {
		t.Errorf("upload to a missing session = %d, want 404", rec.Code)
	}
}

// The work page shows what each source said and how far it can be trusted, so a field that needs a tap is visible
// before she opens the card.
func TestWorkDetailCarriesEverySourceAndItsTrust(t *testing.T) {
	h, _, db := newREST(t)
	ctx := context.Background()
	sources := []domain.Source{
		{
			Provider: "openlibrary", ProviderID: "OL1W", Trust: domain.TrustVerified,
			Citations: []domain.Citation{{URL: "https://example.test/ol", Snippet: "a 1965 novel"}},
		},
		{Provider: "web", ProviderID: "https://example.test/lantern", Trust: domain.TrustUnverified},
	}
	for _, source := range sources {
		if _, err := db.AddSource(ctx, "w1", source); err != nil {
			t.Fatalf("AddSource: %v", err)
		}
	}
	rec := doJSON(t, h, http.MethodGet, "/api/works/w1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("work detail = %d: %s", rec.Code, rec.Body)
	}
	var detail struct {
		Title   string `json:"title"`
		Sources []struct {
			Provider  string `json:"provider"`
			Trust     string `json:"trust"`
			Citations []struct {
				URL     string `json:"url"`
				Snippet string `json:"snippet"`
			} `json:"citations"`
		} `json:"sources"`
	}
	decode(t, rec, &detail)
	if detail.Title != "The Lantern Keeper" || len(detail.Sources) != 2 {
		t.Fatalf("detail = %+v", detail)
	}
	trust := map[string]string{}
	for _, source := range detail.Sources {
		trust[source.Provider] = source.Trust
		if source.Provider == "openlibrary" && (len(source.Citations) != 1 ||
			source.Citations[0].URL != "https://example.test/ol") {
			t.Errorf("citations = %+v", source.Citations)
		}
	}
	if trust["openlibrary"] != string(domain.TrustVerified) || trust["web"] != string(domain.TrustUnverified) {
		t.Errorf("trust = %v", trust)
	}
}

// The reading screens' three additions to the entries routes: the rating order, the entries on one work, and the
// Deleted list with the way back.

// aShelfEntry confirms a card on a given shelf, which is the only way to make an abandoned or wanted row.
func aShelfEntry(t *testing.T, engine *session.Engine, db *store.Store, title string, completion domain.Completion,
	set map[string]json.RawMessage,
) domain.Entry {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveWork(ctx, domain.Work{
		ID: workIDFor(title), Medium: "book", Title: title, Verified: true,
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.StartDraft(ctx, sess.ID, domain.Entry{WorkID: workIDFor(title)}); err != nil {
		t.Fatal(err)
	}
	set["completion"] = json.RawMessage(`"` + string(completion) + `"`)
	if _, err = engine.EditDraft(ctx, sess.ID, session.Edit{BaseVersion: 0, Set: set}); err != nil {
		t.Fatal(err)
	}
	entry, _, _, err := engine.Confirm(ctx, sess.ID, session.ConfirmRequest{BaseVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

// The shelf is a real filter on the wire, and it is part of what a cursor was made for: a cursor from one shelf is
// refused rather than answering a page from another.
func TestLibraryShelfFilterAndCursor(t *testing.T) {
	h, engine, db := newREST(t)
	finished := aConfirmedEntry(t, engine, db, "The Lantern Keeper", "2026-10-30")
	abandoned := aShelfEntry(t, engine, db, "A Cold Inheritance", domain.CompletionAbandoned,
		map[string]json.RawMessage{
			"body":     json.RawMessage(`"the house was beautiful and everybody in it was unpleasant"`),
			"progress": json.RawMessage(`"two episodes"`),
		})
	wanted := aShelfEntry(t, engine, db, "The Quarry Year", domain.CompletionWantTo,
		map[string]json.RawMessage{})

	var page struct {
		Items      []wireEntry `json:"items"`
		NextCursor *string     `json:"next_cursor"`
	}

	// Happy: the library proper holds what she reviewed, which is finished and abandoned, and not a wish.
	all := doJSON(t, h, http.MethodGet, "/api/entries", nil)
	if all.Code != http.StatusOK {
		t.Fatalf("library = %d: %s", all.Code, all.Body)
	}
	decode(t, all, &page)
	if len(page.Items) != 2 {
		t.Fatalf("default shelf = %+v", page.Items)
	}
	for _, item := range page.Items {
		if item.ID == wanted.ID {
			t.Fatal("a wanted row was in the default library")
		}
	}

	// Happy: each shelf is exactly itself, and the abandoned row carries no rating.
	byShelf := doJSON(t, h, http.MethodGet, "/api/entries?completion=abandoned", nil)
	decode(t, byShelf, &page)
	if len(page.Items) != 1 || page.Items[0].ID != abandoned.ID || page.Items[0].RatingQ != 0 {
		t.Fatalf("abandoned shelf = %+v", page.Items)
	}
	if page.Items[0].Progress != "two episodes" {
		t.Fatalf("progress on the wire = %q", page.Items[0].Progress)
	}
	byWanted := doJSON(t, h, http.MethodGet, "/api/entries?completion=want_to", nil)
	decode(t, byWanted, &page)
	if len(page.Items) != 1 || page.Items[0].ID != wanted.ID {
		t.Fatalf("wanted shelf = %+v", page.Items)
	}
	if page.Items[0].Body != "" {
		t.Fatalf("a wish carries a body: %q", page.Items[0].Body)
	}

	// The finished one is still exactly a finished review.
	if finished.Completion != domain.CompletionFinished {
		t.Fatalf("completion = %q", finished.Completion)
	}

	// Bad: a shelf Sepia does not know is refused with a stable code, not silently ignored.
	if bad := doJSON(t, h, http.MethodGet, "/api/entries?completion=halfway", nil); bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown shelf = %d, want 400", bad.Code)
	}

	// Bad: a rating cursor is four keys now. A three-key cursor from another build is refused rather than read as a
	// different position, which is how a page would skip or repeat a row.
	rated := doJSON(t, h, http.MethodGet, "/api/entries?sort=rating&limit=1", nil)
	if rated.Code != http.StatusOK {
		t.Fatalf("rating page = %d: %s", rated.Code, rated.Body)
	}
	decode(t, rated, &page)
	if page.NextCursor == nil {
		t.Fatal("a one-row page of three has no cursor")
	}
	next := doJSON(t, h, http.MethodGet, "/api/entries?sort=rating&limit=1&cursor="+*page.NextCursor, nil)
	if next.Code != http.StatusOK {
		t.Fatalf("second page = %d: %s", next.Code, next.Body)
	}
	var second struct {
		Items []wireEntry `json:"items"`
	}
	decode(t, next, &second)
	if len(second.Items) != 1 || second.Items[0].ID == page.Items[0].ID {
		t.Fatalf("second page = %+v, first page was %+v", second.Items, page.Items)
	}
	// Bad: the rating order used to carry three keys. A cursor in the old shape belongs to a build that no longer
	// exists, so it is refused rather than half-read: reading it as the new shape would skip or repeat a row.
	signature := cursorSignature("entries", "", "any", "", "", string(domain.SortRating), "false")
	oldShape := encodeCursor(signature, "18", "2026-11-02T12:00:00Z", "id-1")
	stale := doJSON(t, h, http.MethodGet, "/api/entries?sort=rating&cursor="+oldShape, nil)
	if stale.Code != http.StatusBadRequest {
		t.Fatalf("old three-key cursor = %d, want 400", stale.Code)
	}
}

func TestLibrarySortWorkAndDeleted(t *testing.T) {
	h, engine, db := newREST(t)
	low := aConfirmedEntry(t, engine, db, "The Lantern Keeper", "2026-10-30")
	high := aConfirmedEntry(t, engine, db, "The Quarry Line", "2026-10-31")
	// A second entry on the same work, so "also on this work" has something to return.
	again := aConfirmedEntry(t, engine, db, "The Lantern Keeper", "2026-11-02")

	var page struct {
		Items      []wireEntry `json:"items"`
		NextCursor *string     `json:"next_cursor"`
	}

	// Happy: the rating order puts the best first, and every row is there.
	byRating := doJSON(t, h, http.MethodGet, "/api/entries?sort=rating", nil)
	if byRating.Code != http.StatusOK {
		t.Fatalf("rating sort = %d: %s", byRating.Code, byRating.Body)
	}
	decode(t, byRating, &page)
	if len(page.Items) != 3 || page.Items[0].RatingQ < page.Items[1].RatingQ {
		t.Fatalf("rating order = %+v", page.Items)
	}

	// Happy: one work's timeline, which is what the entry page's "also on this work" reads.
	onWork := doJSON(t, h, http.MethodGet, "/api/entries?work="+workIDFor("The Lantern Keeper"), nil)
	if onWork.Code != http.StatusOK {
		t.Fatalf("work filter = %d: %s", onWork.Code, onWork.Body)
	}
	decode(t, onWork, &page)
	if len(page.Items) != 2 || page.Items[0].ID != again.ID || page.Items[1].ID != low.ID {
		t.Fatalf("entries on the work = %+v", page.Items)
	}
	if onWork := doJSON(t, h, http.MethodGet, "/api/entries?work="+workIDFor("The Quarry Line"), nil); onWork.Code == http.StatusOK {
		decode(t, onWork, &page)
		if len(page.Items) != 1 || page.Items[0].ID != high.ID {
			t.Fatalf("the other work's timeline = %+v", page.Items)
		}
	}

	// Sad: a deleted entry leaves the library, is still readable by id, and is the Deleted list's only row.
	if del := doJSON(t, h, http.MethodPost, "/api/entries/"+low.ID+"/delete", nil); del.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", del.Code, del.Body)
	}
	library := doJSON(t, h, http.MethodGet, "/api/entries", nil)
	decode(t, library, &page)
	if len(page.Items) != 2 {
		t.Fatalf("library after delete = %+v", page.Items)
	}
	gone := doJSON(t, h, http.MethodGet, "/api/entries/"+low.ID, nil)
	if gone.Code != http.StatusOK {
		t.Fatalf("a deleted entry by id = %d, want 200 so the page can say so", gone.Code)
	}
	var entry wireEntry
	decode(t, gone, &entry)
	if entry.DeletedAt == "" {
		t.Fatal("the deleted entry did not say it was deleted")
	}
	trash := doJSON(t, h, http.MethodGet, "/api/entries?deleted=true", nil)
	if trash.Code != http.StatusOK {
		t.Fatalf("deleted list = %d: %s", trash.Code, trash.Body)
	}
	decode(t, trash, &page)
	if len(page.Items) != 1 || page.Items[0].ID != low.ID {
		t.Fatalf("deleted list = %+v", page.Items)
	}

	// Happy: restore puts it back where it was, and the Deleted list is empty again.
	if restore := doJSON(t, h, http.MethodPost, "/api/entries/"+low.ID+"/restore", nil); restore.Code != http.StatusNoContent {
		t.Fatalf("restore = %d: %s", restore.Code, restore.Body)
	}
	library = doJSON(t, h, http.MethodGet, "/api/entries", nil)
	decode(t, library, &page)
	if len(page.Items) != 3 {
		t.Fatalf("library after restore = %+v", page.Items)
	}
	trash = doJSON(t, h, http.MethodGet, "/api/entries?deleted=true", nil)
	decode(t, trash, &page)
	if len(page.Items) != 0 {
		t.Fatalf("deleted list after restore = %+v", page.Items)
	}

	// Bad: restoring something that was never deleted, and restoring nothing at all.
	if rec := doJSON(t, h, http.MethodPost, "/api/entries/"+low.ID+"/restore", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("restore a live entry = %d, want 400", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/entries/no-such/restore", nil); rec.Code != http.StatusNotFound {
		t.Errorf("restore a missing entry = %d, want 404", rec.Code)
	}
}

// Bad: a cursor belongs to the query that made it. Reusing one from another sort, or another set of filters, must be
// refused rather than answering a page from somewhere else.
func TestCursorFromAnotherQueryIsRefused(t *testing.T) {
	h, engine, db := newREST(t)
	for _, title := range []string{"The Lantern Keeper", "The Quarry Line", "The Salt Road"} {
		aConfirmedEntry(t, engine, db, title, "2026-10-30")
	}
	var page struct {
		Items      []wireEntry `json:"items"`
		NextCursor *string     `json:"next_cursor"`
	}
	first := doJSON(t, h, http.MethodGet, "/api/entries?limit=1", nil)
	decode(t, first, &page)
	if page.NextCursor == nil {
		t.Fatal("no cursor to reuse")
	}
	cursor := *page.NextCursor

	for name, target := range map[string]string{
		"another sort":      "/api/entries?limit=1&sort=rating&cursor=" + cursor,
		"another filter":    "/api/entries?limit=1&medium=film&cursor=" + cursor,
		"the deleted list":  "/api/entries?limit=1&deleted=true&cursor=" + cursor,
		"another list":      "/api/sessions?limit=1&cursor=" + cursor,
		"a tampered cursor": "/api/entries?limit=1&cursor=not-base64",
		"a short cursor":    "/api/entries?limit=1&cursor=" + "MDAw",
	} {
		t.Run(name, func(t *testing.T) {
			rec := doJSON(t, h, http.MethodGet, target, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s = %d, want 400: %s", name, rec.Code, rec.Body)
			}
		})
	}

	// Bad: the values that are not a filter at all.
	for _, target := range []string{
		"/api/entries?sort=oldest",
		"/api/entries?deleted=maybe",
		"/api/entries?work=" + strings.Repeat("w", 129),
	} {
		if rec := doJSON(t, h, http.MethodGet, target, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", target, rec.Code)
		}
	}
}

// The settings row: the screen reads it, writes it, and the write is the whole object.
func TestSettings(t *testing.T) {
	h, _, _ := newREST(t)

	var got struct {
		Preread bool `json:"preread"`
	}
	initial := doJSON(t, h, http.MethodGet, "/api/settings", nil)
	if initial.Code != http.StatusOK {
		t.Fatalf("settings = %d: %s", initial.Code, initial.Body)
	}
	decode(t, initial, &got)
	if !got.Preread {
		t.Fatal("a fresh database has the pre-read off")
	}

	written := doJSON(t, h, http.MethodPut, "/api/settings", map[string]any{"preread": false})
	if written.Code != http.StatusOK {
		t.Fatalf("write = %d: %s", written.Code, written.Body)
	}
	decode(t, written, &got)
	if got.Preread {
		t.Fatal("the write did not turn the pre-read off")
	}
	again := doJSON(t, h, http.MethodGet, "/api/settings", nil)
	decode(t, again, &got)
	if got.Preread {
		t.Fatal("the switch did not stick")
	}

	// Bad: an unknown field, a missing field, and a write without the origin check.
	if rec := doJSON(t, h, http.MethodPut, "/api/settings", map[string]any{"preread": true, "other": 1}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field = %d, want 400", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPut, "/api/settings", map[string]any{}); rec.Code != http.StatusBadRequest {
		t.Errorf("missing switch = %d, want 400", rec.Code)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, "/api/settings",
		strings.NewReader(`{"preread":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "localhost:8893"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Errorf("a hostile origin wrote the settings: %d", rec.Code)
	}
}

// A draft's row title is the work it is about. It is read with the waiting reason, so the two always describe the
// same draft rather than one being a turn behind.
func TestSessionListNamesTheDraftWork(t *testing.T) {
	h, engine, _ := newREST(t)
	ctx := context.Background()
	sess, err := engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.StartDraft(ctx, sess.ID, domain.Entry{WorkID: "w1"}); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, h, http.MethodGet, "/api/sessions?state=active&kind=entry", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body)
	}
	var page struct {
		Items []wireSession `json:"items"`
	}
	decode(t, rec, &page)
	if len(page.Items) != 1 || page.Items[0].Title != "The Lantern Keeper" {
		t.Fatalf("drafts = %+v, want the work as the row title", page.Items)
	}
	if page.Items[0].WaitingReason == nil || *page.Items[0].WaitingReason != "needs_rating" {
		t.Fatalf("waiting = %v", page.Items[0].WaitingReason)
	}

	one := doJSON(t, h, http.MethodGet, "/api/sessions/"+sess.ID, nil)
	var read wireSession
	decode(t, one, &read)
	if read.Title != "The Lantern Keeper" {
		t.Errorf("one session's title = %q, want the work", read.Title)
	}
}

// A stored cover has to reach the client as a URL it can actually load. The agent writes covers/<provider>-<id>.<ext>
// when it fetches a work; the payload has to name that file, and the server has to serve it, or the card shows its
// stand-in forever and the R2 mirror has nothing to copy.
func TestAWorkWithACoverServesItsURL(t *testing.T) {
	rig := newSocketRig(t)
	defer rig.close()
	ctx := context.Background()

	coverBytes := world.SamplePoster()
	dir := filepath.Join(rig.dataDir, "covers")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmdb-movie-303.png"), coverBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	work := domain.Work{
		ID: "w2", Medium: "film", Title: "The Quarry Line", Verified: true,
		CoverPath: "covers/tmdb-movie-303.png", CoverSource: "tmdb",
	}
	if err := rig.db.SaveWork(ctx, work); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, rig.server.Config.Handler, http.MethodGet, "/api/works/w2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("work detail = %d: %s", rec.Code, rec.Body)
	}
	var detail struct {
		CoverURL string `json:"cover_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.CoverURL != "/covers/tmdb-movie-303.png" {
		t.Fatalf("cover_url = %q, want /covers/tmdb-movie-303.png", detail.CoverURL)
	}

	served := doJSON(t, rig.server.Config.Handler, http.MethodGet, detail.CoverURL, nil)
	if served.Code != http.StatusOK {
		t.Fatalf("%s = %d: the payload named a file the server will not serve", detail.CoverURL, served.Code)
	}
	if !bytes.Equal(served.Body.Bytes(), coverBytes) {
		t.Error("the served cover is not the stored file")
	}
}

// G3 says a cover older than 180 days is refreshed on its next use, and a use is a read. The stale poster keeps being
// served (the library should keep its posters offline) with no-cache, and the work is re-read so the next request finds
// a fresh one.
func TestACoverPastItsWindowIsServedWhileItIsRefreshed(t *testing.T) {
	// The flow rig, not the bare one: this path needs an agent with a finder, because the refresh re-reads the work.
	rig := newFlowRig(t)
	defer rig.close()
	ctx := context.Background()

	dir := filepath.Join(rig.dataDir, "covers")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	name := "tmdb-movie-101.png"
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, world.SamplePoster(), 0o600); err != nil {
		t.Fatal(err)
	}
	old := rig.inner.now().Add(-providers.CoverMaxAge - 24*time.Hour)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}
	work := domain.Work{
		ID: "w3", Medium: "film", Title: "The Lantern Keeper", Verified: true,
		CoverPath: "covers/" + name, CoverSource: "tmdb",
		PrimaryProvider: "tmdb", PrimaryID: "movie:101",
	}
	if err := rig.db.SaveWork(ctx, work); err != nil {
		t.Fatal(err)
	}

	// A stale cover is still served: the board is offline most of the time and the library should keep its posters.
	rec := doJSON(t, rig.server.Config.Handler, http.MethodGet, "/covers/"+name, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stale cover = %d, want 200 while the refresh runs", rec.Code)
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "no-cache" {
		t.Errorf("stale cover Cache-Control = %q, want no-cache so the browser revalidates", cache)
	}

	// The refresh runs in the background, so wait for it rather than sleeping a fixed time. The refresh writes the
	// file with the provider client's clock, which in this rig is the same fixed clock.
	deadline := time.Now().Add(5 * time.Second)
	for {
		info, statErr := os.Stat(file)
		if statErr == nil && rig.inner.now().Sub(info.ModTime()) < providers.CoverMaxAge {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the stale cover was never refreshed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if rec := doJSON(t, rig.server.Config.Handler, http.MethodGet, "/covers/"+name, nil); rec.Code != http.StatusOK {
		t.Fatalf("refreshed cover = %d, want 200", rec.Code)
	}
}

// A fresh cover costs no provider call at all: the age check is the whole of the freshness rule on this path.
func TestAFreshCoverIsServedWithoutAskingTheProvider(t *testing.T) {
	rig := newSocketRig(t)
	defer rig.close()

	dir := filepath.Join(rig.dataDir, "covers")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	name := "tmdb-movie-404.png"
	cover := world.SamplePoster()
	if err := os.WriteFile(filepath.Join(dir, name), cover, 0o600); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, rig.server.Config.Handler, http.MethodGet, "/covers/"+name, nil)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), cover) {
		t.Fatalf("fresh cover = %d %q", rec.Code, rec.Body.String())
	}
}

// A cover that cannot be refreshed must not be retried on every read. The realistic case is a poster the provider has
// stopped serving: the fetch succeeds, the image download fails, the file stays stale, and without a retry window every
// render of that page becomes another pair of provider calls. One attempt, then quiet until the window passes.
func TestAFailedRefreshIsNotRetriedOnEveryRead(t *testing.T) {
	rig := newFlowRig(t)
	defer rig.close()
	ctx := context.Background()

	dir := filepath.Join(rig.dataDir, "covers")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	name := "tmdb-movie-101.png"
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, world.SamplePoster(), 0o600); err != nil {
		t.Fatal(err)
	}
	old := rig.inner.now().Add(-providers.CoverMaxAge - 24*time.Hour)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}
	// The provider still answers, and the poster it names is gone: the refresh fails and the file stays stale.
	rig.tmdb.SetFault("/t/p/w92/lantern.jpg", world.FaultError)
	work := domain.Work{
		ID: "w-retry", Medium: "film", Title: "The Lantern Keeper", Verified: true,
		CoverPath: "covers/" + name, CoverSource: "tmdb",
		PrimaryProvider: "tmdb", PrimaryID: "movie:101",
	}
	if err := rig.db.SaveWork(ctx, work); err != nil {
		t.Fatal(err)
	}

	for range 3 {
		if rec := doJSON(t, rig.server.Config.Handler, http.MethodGet, "/covers/"+name, nil); rec.Code != http.StatusOK {
			t.Fatalf("stale cover = %d, want 200", rec.Code)
		}
		// Give a triggered refresh time to reach the world before the next read.
		time.Sleep(150 * time.Millisecond)
	}
	downloads := 0
	for _, request := range rig.tmdb.Requests() {
		if strings.HasSuffix(request.Path, "lantern.jpg") {
			downloads++
		}
	}
	if got := downloads; got != 1 {
		t.Errorf("cover downloads = %d after three reads, want 1: a failed refresh was retried on every read", got)
	}
}

// What enforces G3's cap is the purge, not withholding an image that is still on disk: when the provider answers that
// the cover is gone, the file goes and the row stops naming it, so no TMDB content outlives the six months.
func TestACoverTheProviderDisownsIsPurged(t *testing.T) {
	rig := newFlowRig(t)
	defer rig.close()
	ctx := context.Background()

	dir := filepath.Join(rig.dataDir, "covers")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	name := "tmdb-movie-101.png"
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, world.SamplePoster(), 0o600); err != nil {
		t.Fatal(err)
	}
	old := rig.inner.now().Add(-providers.CoverMaxAge - 24*time.Hour)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}
	// The provider has never heard of this id: a not-found is an answer, not an outage.
	rig.tmdb.SetFault("/movie/101", world.FaultNotFound)
	work := domain.Work{
		ID: "w-gone", Medium: "film", Title: "The Lantern Keeper", Verified: true,
		CoverPath: "covers/" + name, CoverSource: "tmdb",
		PrimaryProvider: "tmdb", PrimaryID: "movie:101",
	}
	if err := rig.db.SaveWork(ctx, work); err != nil {
		t.Fatal(err)
	}

	if rec := doJSON(t, rig.server.Config.Handler, http.MethodGet, "/covers/"+name, nil); rec.Code != http.StatusOK {
		t.Fatalf("stale cover = %d, want 200 while the refresh runs", rec.Code)
	}
	// Poll the row, not just the file: the purge removes the file and then clears the reference, so a read between the
	// two sees a missing file that the row still names. On a loaded runner that window is real.
	var got domain.Work
	deadline := time.Now().Add(5 * time.Second)
	for {
		current, err := rig.db.Work(ctx, work.ID)
		if err != nil {
			t.Fatalf("Work: %v", err)
		}
		got = current
		_, statErr := os.Stat(file)
		if got.CoverPath == "" && errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			if !errors.Is(statErr, os.ErrNotExist) {
				t.Fatal("the disowned cover was never deleted")
			}
			t.Fatalf("cover path = %q, want it cleared once the file is gone", got.CoverPath)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got.CoverPath != "" {
		t.Errorf("cover path = %q, want it cleared once the file is gone", got.CoverPath)
	}
}

// searchPage is the search response as the client reads it.
type searchPage struct {
	Items []wireSearchHit `json:"items"`
	Total int             `json:"total"`
}

// Happy: a phrase from her body finds the entry, with the sentence around it, the match marked, and the entry the row
// draws; the title finds it too; and the total is every match rather than the page.
func TestSearchFindsHerWords(t *testing.T) {
	h, engine, db := newREST(t)
	light := aReviewedEntry(t, engine, db, "The Lantern Keeper", "book",
		"a quiet book about light and memory", 18, "2026-10-30")
	aReviewedEntry(t, engine, db, "Piranesi", "book", "a book that started slow but then it grabbed me", 19,
		"2026-09-28")

	rec := doJSON(t, h, http.MethodGet, "/api/search?q=memory", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("search = %d: %s", rec.Code, rec.Body)
	}
	var page searchPage
	decode(t, rec, &page)
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page = %+v", page)
	}
	hit := page.Items[0]
	if hit.ID != light.ID || hit.Kind != "entry" || hit.Title != "The Lantern Keeper" {
		t.Fatalf("hit = %+v", hit)
	}
	if hit.Entry == nil || hit.Entry.ID != light.ID || hit.Entry.Work.Title != "The Lantern Keeper" {
		t.Fatalf("hit has no entry to draw: %+v", hit.Entry)
	}
	if !strings.Contains(hit.Snippet, "\uE000memory\uE001") {
		t.Fatalf("snippet = %q, want the matched word marked", hit.Snippet)
	}
	if hit.Score <= 0 {
		t.Fatalf("score = %v, want a bigger number for a better match", hit.Score)
	}

	// A title is searchable, and the match is found in it.
	byTitle := doJSON(t, h, http.MethodGet, "/api/search?q=lantern", nil)
	decode(t, byTitle, &page)
	if page.Total != 1 || page.Items[0].ID != light.ID {
		t.Fatalf("by title = %+v", page)
	}

	// The count is every match rather than the page it fitted on.
	all := doJSON(t, h, http.MethodGet, "/api/search?q=book&limit=1", nil)
	decode(t, all, &page)
	if page.Total != 2 || len(page.Items) != 1 {
		t.Fatalf("book = %d matches over %d rows, want 2 over 1", page.Total, len(page.Items))
	}
}

// Bad: an empty search, a search over the length limit, one with too many words, and a bad page size are all refused
// with plain copy. An operator-shaped search is asked as words and answers nothing rather than erroring.
func TestSearchRefusesBadQueries(t *testing.T) {
	h, engine, db := newREST(t)
	aReviewedEntry(t, engine, db, "The Lantern Keeper", "book", "a quiet book about light", 18, "2026-10-30")

	for name, target := range map[string]string{
		"empty":        "/api/search?q=",
		"spaces":       "/api/search?q=%20%20",
		"operators":    "/api/search?q=%22%20*%20(%20%5E",
		"too long":     "/api/search?q=" + strings.Repeat("a", 201),
		"too many":     "/api/search?q=one+two+three+four+five+six+seven+eight+nine",
		"a bad page":   "/api/search?q=light&limit=0",
		"a page again": "/api/search?q=light&limit=nonsense",
	} {
		t.Run(name, func(t *testing.T) {
			rec := doJSON(t, h, http.MethodGet, target, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s = %d, want 400: %s", name, rec.Code, rec.Body)
			}
		})
	}

	// A query shaped like FTS5 syntax is words, not syntax: it is answered, and it matches nothing.
	hostile := doJSON(t, h, http.MethodGet, "/api/search?q=%22+OR+1%3D1+--", nil)
	if hostile.Code != http.StatusOK {
		t.Fatalf("hostile search = %d: %s", hostile.Code, hostile.Body)
	}
	var page searchPage
	decode(t, hostile, &page)
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("hostile search matched something: %+v", page)
	}
}

// Happy: the library's total is the filters', not the page's, and the rating and date filters narrow it.
func TestLibraryTotalAndFilters(t *testing.T) {
	h, engine, db := newREST(t)
	aReviewedEntry(t, engine, db, "The Lantern Keeper", "book", "light and memory", 18, "2026-10-30")
	aReviewedEntry(t, engine, db, "Blade Runner 2049", "film", "slow and deliberate", 19, "2026-10-27")
	aReviewedEntry(t, engine, db, "Piranesi", "book", "started slow", 15, "2026-09-28")

	type page struct {
		Items []wireEntry `json:"items"`
		Total int         `json:"total"`
	}
	read := func(target string) page {
		t.Helper()
		rec := doJSON(t, h, http.MethodGet, target, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", target, rec.Code, rec.Body)
		}
		var got page
		decode(t, rec, &got)
		return got
	}

	if got := read("/api/entries?limit=1"); len(got.Items) != 1 || got.Total != 3 {
		t.Fatalf("total = %d with %d rows, want 3 in all", got.Total, len(got.Items))
	}
	// The total does not change with the page.
	if got := read("/api/entries?limit=2"); got.Total != 3 {
		t.Fatalf("total on the second page size = %d", got.Total)
	}
	if got := read("/api/entries?rating_min=18"); got.Total != 2 {
		t.Fatalf("rating_min total = %d, want 2", got.Total)
	}
	if got := read("/api/entries?rating_max=18"); got.Total != 2 {
		t.Fatalf("rating_max total = %d, want 2", got.Total)
	}
	if got := read("/api/entries?finished_from=2026-10-01"); got.Total != 2 {
		t.Fatalf("finished_from total = %d, want 2", got.Total)
	}
	if got := read("/api/entries?finished_to=2026-09-30"); got.Total != 1 {
		t.Fatalf("finished_to total = %d, want 1", got.Total)
	}
	if got := read("/api/entries?medium=film"); got.Total != 1 {
		t.Fatalf("medium total = %d, want 1", got.Total)
	}

	// Bad: a bound that is not a rating, a date that does not exist, and a range the wrong way round.
	for _, target := range []string{
		"/api/entries?rating_min=0",
		"/api/entries?rating_max=21",
		"/api/entries?rating_min=nine",
		"/api/entries?finished_from=2026-02-31",
		"/api/entries?finished_to=yesterday",
		"/api/entries?rating_min=19&rating_max=17",
		"/api/entries?finished_from=2026-10-01&finished_to=2026-09-01",
	} {
		if rec := doJSON(t, h, http.MethodGet, target, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", target, rec.Code)
		}
	}
}

// Bad: the Deleted list's total is its own. Its shelf rule is not the library's ("everything but a wish"), because a
// deleted wish is still something Restore should find, so the count has to come from the same query the rows do.
func TestDeletedListTotalMatchesItsRows(t *testing.T) {
	h, engine, db := newREST(t)
	ctx := context.Background()
	review := aReviewedEntry(t, engine, db, "Piranesi", "book", "started slow", 17, "2026-09-28")
	if err := db.SaveWork(ctx, domain.Work{ID: "w-wish", Medium: "book", Title: "House of Leaves"}); err != nil {
		t.Fatalf("wish work: %v", err)
	}
	if err := db.CreateDraft(ctx, domain.Entry{
		ID: "e-wish", WorkID: "w-wish", RatingScale: domain.RatingScale, Body: "meant to read it",
		BodyOrigin: domain.BodyTyped, Status: domain.StatusSaved, Completion: domain.CompletionWantTo,
	}); err != nil {
		t.Fatalf("wish: %v", err)
	}
	for _, id := range []string{review.ID, "e-wish"} {
		if rec := doJSON(t, h, http.MethodPost, "/api/entries/"+id+"/delete", nil); rec.Code != http.StatusNoContent {
			t.Fatalf("delete %s = %d: %s", id, rec.Code, rec.Body)
		}
	}

	type page struct {
		Items []wireEntry `json:"items"`
		Total int         `json:"total"`
	}
	read := func(target string) page {
		t.Helper()
		rec := doJSON(t, h, http.MethodGet, target, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", target, rec.Code, rec.Body)
		}
		var got page
		decode(t, rec, &got)
		return got
	}

	all := read("/api/entries?deleted=true")
	if len(all.Items) != 2 || all.Total != 2 {
		t.Fatalf("deleted = %d rows, total %d, want 2 and 2", len(all.Items), all.Total)
	}
	finished := read("/api/entries?deleted=true&completion=finished")
	if len(finished.Items) != 1 || finished.Total != 1 {
		t.Fatalf("deleted finished = %d rows, total %d, want 1 and 1", len(finished.Items), finished.Total)
	}
}

// A candidate's cover is drawn from the app: the page's CSP is img-src 'self' data:, and the provider's image host is
// not 'self', so the picker's <img> never loaded and every candidate showed its initials (issue #53). The payload
// names this route, and the app fetches the provider's copy through the same cache and limits a saved cover uses.
func TestACandidateThumbIsFetchedAndServedFromThisOrigin(t *testing.T) {
	rig := newFlowRig(t)
	defer rig.close()
	handler := rig.server.Config.Handler
	remote := rig.tmdb.URL() + "/t/p/w92/lantern.jpg"
	path := providers.ThumbPath(providers.ProviderTMDB, remote)

	rec := doJSON(t, handler, http.MethodGet, path, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("thumb = %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content type = %q, want the stored file's", ct)
	}
	if !bytes.Equal(rec.Body.Bytes(), world.SamplePoster()) {
		t.Error("the served thumb is not the provider's image")
	}
	// Its own name in the cache, derived from the source: the work's cover will take the plain one, and a thumb must
	// not stand in for it.
	stored, err := os.ReadDir(filepath.Join(rig.dataDir, providers.CoverDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || !strings.HasPrefix(stored[0].Name(), "tmdb-thumb-") {
		t.Fatalf("covers = %v, want the one thumb under its own name", stored)
	}
	if calls := rig.tmdb.Calls("/t/p/w92/lantern.jpg"); calls != 1 {
		t.Fatalf("provider calls = %d, want one", calls)
	}

	// A redraw is the stored copy, not a second download: a picker that draws again must not spend provider calls.
	again := doJSON(t, handler, http.MethodGet, path, nil)
	if again.Code != http.StatusOK {
		t.Fatalf("second thumb = %d: %s", again.Code, again.Body)
	}
	if calls := rig.tmdb.Calls("/t/p/w92/lantern.jpg"); calls != 1 {
		t.Errorf("provider calls after a redraw = %d, want the stored copy", calls)
	}
}

// The thumb route is not an open image fetch. A host the config does not name is refused even over https, a provider
// name is not a path, and a request with no source fetches nothing at all.
func TestAThumbIsRefusedUnlessItIsFromAConfiguredHost(t *testing.T) {
	rig := newFlowRig(t)
	defer rig.close()
	handler := rig.server.Config.Handler

	for name, path := range map[string]string{
		"a host the config does not name": providers.ThumbPath(
			providers.ProviderTMDB, "https://example.test/x.jpg"),
		"a plain-http host": providers.ThumbPath(
			providers.ProviderTMDB, "http://192.0.2.10/x.jpg"),
		"a provider that is not one it knows": providers.ThumbPath(
			"../../etc", rig.tmdb.URL()+"/t/p/w92/lantern.jpg"),
		"no source at all": "/api/thumb?provider=tmdb",
	} {
		if rec := doJSON(t, handler, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404: %s", name, rec.Code, rec.Body)
		}
	}

	// A refusal writes nothing, and the route is a read: a thumb is never posted.
	entries, err := os.ReadDir(filepath.Join(rig.dataDir, providers.CoverDir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a refused thumb left %d file(s) in the cover cache", len(entries))
	}
	post := doJSON(t, handler, http.MethodPost, "/api/thumb?provider=tmdb", map[string]any{})
	if post.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", post.Code)
	}
}

// A provider that has stopped serving an image costs the candidate its cover and nothing else, and leaves no file.
func TestAThumbTheProviderDoesNotHaveIsA404(t *testing.T) {
	rig := newFlowRig(t)
	defer rig.close()
	path := providers.ThumbPath(providers.ProviderTMDB, rig.tmdb.URL()+"/t/p/w92/missing.jpg")

	if rec := doJSON(t, rig.server.Config.Handler, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing thumb = %d, want 404: %s", rec.Code, rec.Body)
	}
	entries, err := os.ReadDir(filepath.Join(rig.dataDir, providers.CoverDir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a missing thumb left %d file(s)", len(entries))
	}
}

// Another service on the same machine is not a cover origin, even though it shares the loopback host the dev cover base
// names. Without that rule the thumb route is a client-driven fetch of whatever is listening locally, which is what the
// review of #53 reproduced.
func TestAThumbCannotBeAnotherLocalService(t *testing.T) {
	rig := newFlowRig(t)
	defer rig.close()

	var asked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(world.SamplePoster())
	}))
	defer other.Close()

	path := providers.ThumbPath(providers.ProviderTMDB, other.URL+"/secret.png")
	if rec := doJSON(t, rig.server.Config.Handler, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("thumb from another local service = %d, want 404: %s", rec.Code, rec.Body)
	}
	if n := asked.Load(); n != 0 {
		t.Errorf("the other local service was asked %d time(s), want none", n)
	}
}
