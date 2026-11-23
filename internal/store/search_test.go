package store

import (
	"context"
	"strings"
	"testing"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// Search is her own words found again. These tests drive the real index: the triggers that keep it in step, the
// ranking between title, body and answers, the filters, and the one failure the triggers cannot cover, an index that
// was edited by hand or restored from an older file.

// searchable writes a work and one saved entry with the title, body and shelf a case needs. The answers are added
// afterwards, because that is the order a real entry is written in: the card is confirmed and then its answers land.
func searchable(t *testing.T, s *Store, id, title, body string, medium string, rating int, finishedOn string) {
	t.Helper()
	work := sampleWork("w-" + id)
	work.ID, work.Title, work.Medium = "w-"+id, title, medium
	if err := s.SaveWork(context.Background(), work); err != nil {
		t.Fatalf("work %s: %v", id, err)
	}
	entry := sampleEntry(id, work.ID, rating)
	entry.Status = domain.StatusSaved
	entry.Body = body
	entry.FinishedOn = finishedOn
	entry.Completion = domain.CompletionFinished
	if err := s.CreateDraft(context.Background(), entry); err != nil {
		t.Fatalf("entry %s: %v", id, err)
	}
}

func answer(t *testing.T, s *Store, entryID, questionID string, values ...string) {
	t.Helper()
	err := s.AddAnswers(context.Background(), entryID, []domain.Answer{{
		QuestionID: questionID, Category: "mood", Prompt: "How did it feel?",
		Type: domain.AnswerTags, Values: values,
	}})
	if err != nil {
		t.Fatalf("answer %s: %v", entryID, err)
	}
}

func hitIDs(hits []domain.SearchHit) string {
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}
	return strings.Join(ids, ",")
}

func search(t *testing.T, s *Store, text string) ([]domain.SearchHit, int) {
	t.Helper()
	hits, total, err := s.Search(context.Background(), domain.SearchQuery{Text: text, Limit: 50})
	if err != nil {
		t.Fatalf("Search(%q): %v", text, err)
	}
	return hits, total
}

// Happy: a phrase in her body finds the entry, with the sentence around it and the matched word marked.
func TestSearchFindsHerBodyWithASnippet(t *testing.T) {
	s := openTest(t)
	searchable(t, s, "e1", "Piranesi", "started slow but then it grabbed me and would not let go", "book", 19, "2026-09-28")

	hits, total := search(t, s, "slow")
	if total != 1 || len(hits) != 1 {
		t.Fatalf("hits = %+v, total = %d", hits, total)
	}
	hit := hits[0]
	if hit.ID != "e1" || hit.Entry.ID != "e1" || hit.Work.Title != "Piranesi" || hit.Kind != "entry" {
		t.Fatalf("hit = %+v", hit)
	}
	// The snippet wraps the match itself, and carries the words around it rather than the whole body.
	if !strings.Contains(hit.Snippet, domain.SnippetOpen+"slow"+domain.SnippetClose) {
		t.Fatalf("snippet = %q, want the matched word marked", hit.Snippet)
	}
	if strings.Contains(hit.Snippet, "grabbed me") && strings.Contains(hit.Snippet, "Piranesi") {
		t.Fatalf("snippet = %q, want a window rather than the whole body and title", hit.Snippet)
	}
	if hit.Score <= 0 {
		t.Fatalf("score = %v, want a positive score where a bigger number is a better match", hit.Score)
	}
}

// Happy: a chip she picked is searchable, and a title is too. The ranking is title, then body, then answers
// (docs/slices/S16-search-and-chat.md, Decisions 1).
func TestSearchRanksTitleBeforeBodyBeforeAnswers(t *testing.T) {
	s := openTest(t)
	searchable(t, s, "e-title", "Slow Horses", "a spy novel that never stops moving", "book", 17, "2026-10-01")
	searchable(t, s, "e-body", "The Night Circus", "a bit too slow in places but the atmosphere is unmatched", "book", 16, "2026-03-12")
	searchable(t, s, "e-answers", "Severance", "the office and the outside world never quite meet", "tv", 18, "2026-09-14")
	answer(t, s, "e-answers", "pace", "slow")

	hits, total := search(t, s, "slow")
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if got := hitIDs(hits); got != "e-title,e-body,e-answers" {
		t.Fatalf("order = %s, want the title first, then the body, then the answer", got)
	}
	// The answer's hit is marked in the answers column: the body does not hold it, so an empty snippet would leave
	// the row with nothing to show.
	if !strings.Contains(hits[2].Snippet, domain.SnippetOpen+"slow"+domain.SnippetClose) {
		t.Fatalf("answer snippet = %q", hits[2].Snippet)
	}
}

// Happy: the index follows every write the triggers cover, in both directions.
func TestSearchIndexFollowsWrites(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	searchable(t, s, "e1", "Dune", "the world felt so lived in", "book", 18, "2026-10-30")

	if hits, _ := search(t, s, "lived"); len(hits) != 1 {
		t.Fatalf("before the answer: %+v", hits)
	}
	// An answer she gives after the card is confirmed is searchable immediately.
	answer(t, s, "e1", "mood", "vast")
	if hits, _ := search(t, s, "vast"); len(hits) != 1 {
		t.Fatalf("after the answer: %+v", hits)
	}
	// A soft delete takes the entry out of search, and a restore puts it back.
	if err := s.SoftDeleteEntry(ctx, "e1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if hits, total := search(t, s, "lived"); len(hits) != 0 || total != 0 {
		t.Fatalf("deleted entry is still indexed: %+v", hits)
	}
	if err := s.RestoreEntry(ctx, "e1"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if hits, _ := search(t, s, "lived"); len(hits) != 1 {
		t.Fatalf("restored entry is missing: %+v", hits)
	}
	// A corrected title is the one she will look for.
	work, err := s.Work(ctx, "w-e1")
	if err != nil {
		t.Fatalf("work: %v", err)
	}
	work.Title = "Dune Messiah"
	if err := s.SaveWork(ctx, work); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if hits, _ := search(t, s, "messiah"); len(hits) != 1 {
		t.Fatalf("renamed title is not indexed: %+v", hits)
	}
}

// Bad: a draft is not in the library, so it is not in search either, and confirming it is what indexes it.
func TestSearchIndexesOnlySavedEntries(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	work := sampleWork("w1")
	if err := s.SaveWork(ctx, work); err != nil {
		t.Fatal(err)
	}
	draft := sampleEntry("e1", "w1", 18)
	draft.Body = "a draft nobody has confirmed yet"
	if err := s.CreateDraft(ctx, draft); err != nil {
		t.Fatal(err)
	}
	if hits, total := search(t, s, "confirmed"); len(hits) != 0 || total != 0 {
		t.Fatalf("a draft is searchable: %+v", hits)
	}
	draft.Status = domain.StatusSaved
	if _, err := s.SaveEntry(ctx, draft); err != nil {
		t.Fatal(err)
	}
	if hits, _ := search(t, s, "confirmed"); len(hits) != 1 {
		t.Fatalf("a confirmed entry is not searchable: %+v", hits)
	}
}

// Bad: FTS5 syntax and SQL-shaped text are asked as words. Nothing is executed, nothing errors, and an operator is
// only ever part of a word, so "^Dune" finds Dune and a query of operators alone is refused as empty.
func TestSearchTreatsHostileTextAsWords(t *testing.T) {
	s := openTest(t)
	searchable(t, s, "e1", "Dune", "the world felt so lived in", "book", 18, "2026-10-30")
	cases := map[string]bool{
		`" OR 1=1 --`:          false,
		`NEAR(lived Dune)`:     false,
		`'); DROP TABLE x; --`: false,
		`Dune*`:                true,
		`^Dune`:                true,
		`dune NEAR/2 lived`:    false,
	}
	for raw, want := range cases {
		hits, _, err := s.Search(context.Background(), domain.SearchQuery{Text: raw, Limit: 50})
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if got := len(hits) > 0; got != want {
			t.Fatalf("%q: %d hits, want a match: %v", raw, len(hits), want)
		}
	}
	// The table the hostile text named is still there, with its row.
	entry, err := s.Entry(context.Background(), "e1")
	if err != nil || entry.ID != "e1" {
		t.Fatalf("the entry is gone: %v", err)
	}
	if _, _, err := s.Search(context.Background(), domain.SearchQuery{Text: " * ", Limit: 10}); err == nil {
		t.Fatal("a search of operators alone was run instead of refused")
	}
}

// Bad: a refused search comes back as the parser's sentinel, so the server can say why.
func TestSearchRefusesAnEmptyQuery(t *testing.T) {
	s := openTest(t)
	if _, _, err := s.Search(context.Background(), domain.SearchQuery{Text: "  -- ", Limit: 10}); err == nil {
		t.Fatal("an empty search was answered")
	}
}

// Happy: the count is the number of matches, not the number on the page.
func TestSearchCountsPastThePage(t *testing.T) {
	s := openTest(t)
	for i, id := range []string{"e1", "e2", "e3"} {
		searchable(t, s, id, "Book "+id, "a slow read, honestly", "book", 15+i, "2026-10-0"+string(rune('1'+i)))
	}
	hits, total, err := s.Search(context.Background(), domain.SearchQuery{Text: "slow", Limit: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || total != 3 {
		t.Fatalf("hits = %d, total = %d, want one hit out of three", len(hits), total)
	}
}

// Happy: the filters narrow the index by the same things the library list is narrowed by.
func TestSearchFilters(t *testing.T) {
	s := openTest(t)
	searchable(t, s, "e-film", "Blade Runner 2049", "slow and deliberate", "film", 19, "2026-10-27")
	searchable(t, s, "e-book", "Piranesi", "slow to start", "book", 17, "2026-09-28")
	ctx := context.Background()

	cases := map[string]domain.SearchQuery{
		"medium":     {Text: "slow", Medium: "book"},
		"rating min": {Text: "slow", RatingMin: 18},
		"rating max": {Text: "slow", RatingMax: 18},
		"date from":  {Text: "slow", From: "2026-10-01"},
		"date to":    {Text: "slow", To: "2026-09-30"},
	}
	want := map[string]string{
		"medium": "e-book", "rating min": "e-film", "rating max": "e-book",
		"date from": "e-film", "date to": "e-book",
	}
	for name, query := range cases {
		query.Limit = 50
		hits, total, err := s.Search(ctx, query)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := hitIDs(hits); got != want[name] || total != 1 {
			t.Fatalf("%s: hits = %s total = %d, want %s", name, got, total, want[name])
		}
	}
	// A wanted work is on her list, so search finds it: "have I already got this?" is the question it answers most.
	wanted := sampleEntry("e-want", "w-e-book", 0)
	wanted.Completion = domain.CompletionWantTo
	wanted.Status = domain.StatusSaved
	wanted.Body = "meant to read this for ages"
	if err := s.CreateDraft(ctx, wanted); err != nil {
		t.Fatalf("wanted: %v", err)
	}
	if hits, _ := search(t, s, "ages"); len(hits) != 1 || hits[0].ID != "e-want" {
		t.Fatalf("a wanted work is not searchable: %+v", hits)
	}
}

// Sad: an index that has lost a row is detected by the counts and rebuilt before the search is answered, so a restore
// from an older file or a hand-edited index is not a silently missing review.
func TestSearchRepairsAnIndexOutOfStep(t *testing.T) {
	s := openTest(t)
	searchable(t, s, "e1", "Dune", "the world felt so lived in", "book", 18, "2026-10-30")
	if err := raw(t, s, `DELETE FROM entries_fts`); err != nil {
		t.Fatalf("empty the index: %v", err)
	}
	hits, total, err := s.Search(context.Background(), domain.SearchQuery{Text: "lived", Limit: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || total != 1 {
		t.Fatalf("hits = %+v, total = %d, want the entry found after the repair", hits, total)
	}
	drifted, err := s.SearchIndexDrifted(context.Background())
	if err != nil {
		t.Fatalf("SearchIndexDrifted: %v", err)
	}
	if drifted {
		t.Fatal("the index is still out of step after a search repaired it")
	}
}

// Sad: the rebuild command rewrites the whole index from the rows and says how many entries it wrote.
func TestRebuildSearchIndex(t *testing.T) {
	s := openTest(t)
	for i, id := range []string{"e1", "e2"} {
		searchable(t, s, id, "Book "+id, "a slow read, honestly", "book", 15+i, "2026-10-0"+string(rune('1'+i)))
	}
	if err := raw(t, s, `DELETE FROM entries_fts`); err != nil {
		t.Fatalf("empty the index: %v", err)
	}
	if err := raw(t, s, `INSERT INTO entries_fts (entry_id, title, body, answers) VALUES ('ghost', 'x', 'x', 'x')`); err != nil {
		t.Fatalf("add a ghost row: %v", err)
	}
	written, err := s.RebuildSearchIndex(context.Background())
	if err != nil {
		t.Fatalf("RebuildSearchIndex: %v", err)
	}
	if written != 2 {
		t.Fatalf("written = %d, want two entries", written)
	}
	if hits, total := search(t, s, "x"); len(hits) != 0 || total != 0 {
		t.Fatalf("the ghost row survived the rebuild: %+v", hits)
	}
	if hits, total := search(t, s, "slow"); len(hits) != 2 || total != 2 {
		t.Fatalf("hits = %+v total = %d", hits, total)
	}
}
