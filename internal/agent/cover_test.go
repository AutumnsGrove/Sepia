package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/providers"
	"github.com/AutumnsGrove/sepia/internal/world"
)

// The cover half of S06 that shipped without a caller: a fetched work becomes a work with a cover on disk, so
// the card's work payload can name one and the R2 mirror has something to copy.
//
// The route that matters most is the sad one. A cover is decoration beside a review she has written, so an image
// host being down must never cost her the card: a failed download drops the cover and nothing else.

// filmScript is the smallest turn that ends with a card on a film: look up, merge the candidate she tapped, save.
func filmScript() []world.ChatTurn {
	return []world.ChatTurn{
		{ToolCalls: []world.ChatToolCall{toolCall("c1", "media_lookup", `{"title":"The Lantern Keeper","medium":"film"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c2", "merge_candidates", `{"candidate_id":"tmdb|movie:101"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c3", "save_entry", `{"body_quotes":["the ending landed but it was too long"],"verdict_quote":"the ending landed"}`)}},
		{Text: "Ready."},
	}
}

// saveOneFilm runs one whole turn on the film sentence and returns the work it stored. Each call is its own
// session, so a second call exercises a second fetch of the same work.
func saveOneFilm(t *testing.T, r *rig) domain.Work {
	t.Helper()
	ctx := context.Background()
	r.responder.script("choose_candidate", "tmdb|movie:101")
	text := "watched the lantern keeper on netflix last night, 3.5/5, the ending landed but it was too long"
	sess, err := r.session.CreateEntry(ctx)
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	out, err := r.agent.HandleMessage(ctx, TurnInput{SessionID: sess.ID, Text: text, ClientMsgID: "m1"})
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if out.Draft == nil {
		t.Fatal("no card: a cover problem must not cost her the card")
	}
	work, err := r.store.Work(ctx, out.Draft.Entry.WorkID)
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	return work
}

// coverOnDisk is the stored cover's full path, or "" when the work has none.
func coverOnDisk(r *rig, work domain.Work) string {
	if work.CoverPath == "" {
		return ""
	}
	return filepath.Join(r.dataDir, filepath.FromSlash(work.CoverPath))
}

// coverRequests counts the downloads of the fixture's poster, which is the only cover path these tests touch.
func coverRequests(srv *world.Server) int {
	n := 0
	for _, req := range srv.Requests() {
		if strings.HasSuffix(req.Path, "lantern.jpg") {
			n++
		}
	}
	return n
}

func TestAFetchedWorkKeepsItsCover(t *testing.T) {
	r := newRig(t, rigOptions{script: filmScript()})
	scriptFilm(r.fake)

	work := saveOneFilm(t, r)
	if work.CoverPath == "" {
		t.Fatal("the work has no cover path: the fetch path does not download one")
	}
	if !strings.HasPrefix(work.CoverPath, providers.CoverDir+"/") {
		t.Errorf("cover path = %q, want it relative to the data directory under %s/", work.CoverPath, providers.CoverDir)
	}
	if work.CoverSource != providers.ProviderTMDB {
		t.Errorf("cover source = %q, want %q", work.CoverSource, providers.ProviderTMDB)
	}
	info, err := os.Stat(coverOnDisk(r, work))
	if err != nil {
		t.Fatalf("the cover file was not written: %v", err)
	}
	if info.Size() == 0 {
		t.Error("the cover file is empty")
	}
}

func TestAFailedCoverLeavesTheWorkWithoutOne(t *testing.T) {
	r := newRig(t, rigOptions{script: filmScript()})
	scriptFilm(r.fake)
	r.tmdb.SetFault("/t/p/w92/lantern.jpg", world.FaultError)

	work := saveOneFilm(t, r)
	if work.CoverPath != "" {
		t.Errorf("cover path = %q, want none: the image host answered 503", work.CoverPath)
	}
	// The download has to have been tried, or this test proves nothing: a path that never asks for a cover also
	// ends up with none.
	if got := coverRequests(r.tmdb); got != 1 {
		t.Errorf("cover requests = %d, want 1: the fetch path did not try the download", got)
	}
	if file := coverOnDisk(r, work); file != "" {
		if _, err := os.Stat(file); err == nil {
			t.Errorf("%s was written even though the download failed", file)
		}
	}
}

func TestASecondAttemptGetsTheCoverAfterTheOutage(t *testing.T) {
	r := newRig(t, rigOptions{script: filmScript()})
	scriptFilm(r.fake)
	r.tmdb.SetFault("/t/p/w92/lantern.jpg", world.FaultError)

	if work := saveOneFilm(t, r); work.CoverPath != "" {
		t.Fatalf("cover path = %q on the first attempt, want none during the outage", work.CoverPath)
	}
	if got := coverRequests(r.tmdb); got != 1 {
		t.Fatalf("cover requests = %d, want 1: the first attempt did not try the download", got)
	}
	r.tmdb.ClearFault("/t/p/w92/lantern.jpg")

	// The same work, fetched again. A failed download is not remembered as a verdict about the cover.
	if work := saveOneFilm(t, r); work.CoverPath == "" {
		t.Error("no cover after the outage cleared: a failed download was treated as final")
	}
}

func TestAStaleCoverIsFetchedAgain(t *testing.T) {
	r := newRig(t, rigOptions{script: filmScript()})
	scriptFilm(r.fake)

	work := saveOneFilm(t, r)
	file := coverOnDisk(r, work)
	if file == "" {
		t.Fatal("no cover to age")
	}
	// 181 days: one day past the cap G3 set, so the next use must fetch it again.
	old := r.clock.Now().Add(-providers.CoverMaxAge - 24*time.Hour)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	before := coverRequests(r.tmdb)

	if got := saveOneFilm(t, r); got.CoverPath == "" {
		t.Fatal("the stale cover was not fetched again")
	}
	if after := coverRequests(r.tmdb); after <= before {
		t.Errorf("cover requests = %d, want more than %d: a cover older than %v must be fetched again",
			after, before, providers.CoverMaxAge)
	}
}

func TestAFreshCoverIsNotFetchedAgain(t *testing.T) {
	r := newRig(t, rigOptions{script: filmScript()})
	scriptFilm(r.fake)

	if work := saveOneFilm(t, r); work.CoverPath == "" {
		t.Fatal("no cover on the first fetch")
	}
	if got := saveOneFilm(t, r); got.CoverPath == "" {
		t.Fatal("the fresh cover was not reused")
	}
	if got := coverRequests(r.tmdb); got != 1 {
		t.Errorf("cover requests = %d, want 1: a cover younger than %v must not be downloaded again",
			got, providers.CoverMaxAge)
	}
}

// The book half: Open Library's covers live at <base>/b/id/<id>-<size>.jpg, so the sample world has to answer
// that shape or every book keeps its stand-in. The film fixture's paths do not prove the book one.
func TestABookFetchStoresItsCover(t *testing.T) {
	r := newRig(t, rigOptions{script: filmScript()})

	result, err := r.finder.Fetch(context.Background(), providers.ProviderOpenLibrary, "OL1W")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result.CoverURL == "" {
		t.Fatal("the sample work has no cover URL")
	}
	got, coverErr := r.agent.attachCover(context.Background(), result)
	if coverErr != nil {
		t.Fatalf("attachCover: %v", coverErr)
	}
	if got.Work.CoverPath == "" {
		t.Fatalf("no cover stored for %s: the world does not serve %s", result.Work.Title, result.CoverURL)
	}
	if _, err := os.Stat(filepath.Join(r.dataDir, filepath.FromSlash(got.Work.CoverPath))); err != nil {
		t.Fatalf("the cover file was not written: %v", err)
	}
}

// A cover already on the work is not erased by a fetch that cannot replace it. The file stays under data/covers/,
// so dropping the path would leave the card on its stand-in with the image sitting right there.
func TestAStaleCoverSurvivesAnOutage(t *testing.T) {
	r := newRig(t, rigOptions{script: filmScript()})
	scriptFilm(r.fake)

	first := saveOneFilm(t, r)
	if first.CoverPath == "" {
		t.Fatal("no cover on the first fetch")
	}
	// Stale, so the next fetch has to try the network again; and the network is down when it does.
	old := r.clock.Now().Add(-providers.CoverMaxAge - 24*time.Hour)
	if err := os.Chtimes(coverOnDisk(r, first), old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	r.tmdb.SetFault("/t/p/w92/lantern.jpg", world.FaultError)

	second := saveOneFilm(t, r)
	if second.CoverPath != first.CoverPath {
		t.Errorf("cover path = %q after a failed refresh, want the stored %q", second.CoverPath, first.CoverPath)
	}
	if _, err := os.Stat(coverOnDisk(r, second)); err != nil {
		t.Errorf("the stored cover went missing: %v", err)
	}
}

// A provider that has stopped advertising a poster must not cost the copy already on disk. Finder.Cover checks the
// stored file before it looks at the URL, so the empty-URL case has to reach it rather than returning early.
func TestAStoredCoverSurvivesThePosterDisappearing(t *testing.T) {
	r := newRig(t, rigOptions{script: filmScript()})
	scriptFilm(r.fake)

	stored := saveOneFilm(t, r)
	if stored.CoverPath == "" {
		t.Fatal("no cover on the first fetch")
	}

	ctx := context.Background()
	result, err := r.finder.Fetch(ctx, providers.ProviderTMDB, "movie:101")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The provider's answer with its poster path removed: the work still comes back, with no cover URL at all.
	result.CoverURL = ""
	got, coverErr := r.agent.attachCover(ctx, result)
	if coverErr != nil {
		t.Fatalf("attachCover: %v", coverErr)
	}
	if got.Work.CoverPath != stored.CoverPath {
		t.Errorf("cover path = %q with no cover URL, want the stored %q", got.Work.CoverPath, stored.CoverPath)
	}
}

// A refresh runs as a background job between two provider round trips, so a newer fetch can land in that window. The
// refresh must write the cover and nothing else, or it reverts the newer metadata to the snapshot it started from.
// This is that race, made deterministic: the row is updated after the refresh's snapshot is taken.
func TestARefreshWritesOnlyTheCover(t *testing.T) {
	r := newRig(t, rigOptions{script: filmScript()})
	ctx := context.Background()

	snapshot := domain.Work{
		ID: "w-race", Medium: "film", Title: "The Lantern Keeper", Year: 2019, Verified: true,
		PrimaryProvider: providers.ProviderTMDB, PrimaryID: "movie:101",
	}
	if err := r.store.SaveWork(ctx, snapshot); err != nil {
		t.Fatalf("SaveWork: %v", err)
	}
	// The newer fetch lands while the refresh is in flight.
	newer := snapshot
	newer.Title = "The Lantern Keeper (restored cut)"
	newer.Year = 2024
	if err := r.store.SaveWork(ctx, newer); err != nil {
		t.Fatalf("SaveWork: %v", err)
	}

	refreshed, err := r.agent.RefreshCover(ctx, snapshot)
	if err != nil {
		t.Fatalf("RefreshCover: %v", err)
	}
	if refreshed.CoverPath == "" {
		t.Fatal("the refresh stored no cover")
	}
	stored, err := r.store.Work(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if stored.CoverPath != refreshed.CoverPath {
		t.Errorf("cover path = %q, want the refreshed %q", stored.CoverPath, refreshed.CoverPath)
	}
	if stored.Title != newer.Title || stored.Year != newer.Year {
		t.Errorf("work = %q (%d), want the newer metadata %q (%d) to survive", stored.Title, stored.Year, newer.Title, newer.Year)
	}
}
