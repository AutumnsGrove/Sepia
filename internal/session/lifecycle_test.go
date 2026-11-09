package session

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/store"
)

// --- revisit and a revision -----------------------------------------------------------------------------

func TestRevisionMovesOntoTheSavedEntry(t *testing.T) {
	e, db, sess := draftSession(t)
	ctx := context.Background()

	// The draft becomes a saved entry first, so there is something to revise.
	draft, err := e.EditDraft(ctx, sess.ID, Edit{BaseVersion: 0, Set: patch(map[string]any{
		"rating": 15, "body": "First thoughts, which I want to change.",
	})})
	if err != nil {
		t.Fatalf("EditDraft: %v", err)
	}
	saved, _, _, err := e.Confirm(ctx, sess.ID, ConfirmRequest{BaseVersion: draft.Version})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	// A new session offers the choice and opens a working copy.
	next, err := e.CreateEntry(ctx)
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	question, err := e.OfferRevisit(ctx, next.ID, saved, "The Lantern Keeper")
	if err != nil {
		t.Fatalf("OfferRevisit: %v", err)
	}
	if question.Kind != "revisit_choice" {
		t.Errorf("question = %+v", question)
	}
	revision, err := e.StartRevision(ctx, next.ID, saved)
	if err != nil {
		t.Fatalf("StartRevision: %v", err)
	}
	if revision.Entry.ID == saved.ID || revision.Entry.Rating != saved.Rating {
		t.Fatalf("working copy = %+v, want a copy with its own id and the same fields", revision.Entry)
	}

	edited, err := e.EditDraft(ctx, next.ID, Edit{BaseVersion: revision.Version, Set: patch(map[string]any{
		"body": "Second thoughts, which are better.",
	})})
	if err != nil {
		t.Fatalf("EditDraft: %v", err)
	}
	entry, _, _, err := e.Confirm(ctx, next.ID, ConfirmRequest{BaseVersion: edited.Version, Revises: saved.ID})
	if err != nil {
		t.Fatalf("Confirm revision: %v", err)
	}
	if entry.ID != saved.ID {
		t.Errorf("entry id = %q, want the saved entry to keep its id", entry.ID)
	}
	if entry.Body != "Second thoughts, which are better." {
		t.Errorf("body = %q", entry.Body)
	}

	// The saved entry is one row with one revision, and the working copy is retired rather than left as a second
	// review of the same work.
	revisions, err := db.Revisions(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Revisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].Snapshot.Body != "First thoughts, which I want to change." {
		t.Errorf("revisions = %+v", revisions)
	}
	library, _, err := db.Library(ctx, domain.Cursor{}, 10)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	for _, row := range library {
		if row.WorkID == "w1" && row.ID != saved.ID {
			t.Errorf("the working copy is in the library: %+v", row)
		}
	}
	copyRow, err := db.Entry(ctx, revision.Entry.ID)
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if !copyRow.Deleted {
		t.Error("the working copy was not retired")
	}
}

func TestStartRevisionRefusesASecondCard(t *testing.T) {
	e, db, sess := draftSession(t)
	if _, err := e.StartRevision(context.Background(), sess.ID, domain.Entry{ID: "e-other", WorkID: "w1"}); err == nil {
		t.Fatal("a session with a card accepted a second one")
	}
	if _, err := db.Session(context.Background(), sess.ID); err != nil {
		t.Fatal(err)
	}
}

// --- rounds and the queue -------------------------------------------------------------------------------

func TestAskRoundStopsAtTheCap(t *testing.T) {
	e, _, sess := draftSession(t)
	ctx := context.Background()
	for i := 0; i < MaxQuestionRounds; i++ {
		if _, err := e.AskRound(ctx, sess.ID, "question_form", map[string]any{"questions": []string{}}); err != nil {
			t.Fatalf("round %d: %v", i+1, err)
		}
	}
	if _, err := e.AskRound(ctx, sess.ID, "question_form", map[string]any{"questions": []string{}}); !errors.Is(err, ErrTooManyRounds) {
		t.Fatalf("round five = %v, want ErrTooManyRounds", err)
	}
	if rounds, err := e.Rounds(ctx, sess.ID); err != nil || rounds != MaxQuestionRounds {
		t.Errorf("rounds = %d, %v", rounds, err)
	}
}

func TestQueueIsOneDeep(t *testing.T) {
	q := NewQueue()
	if err := q.Enqueue("s1"); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := q.Enqueue("s1"); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Enqueue = %v, want ErrBusy", err)
	}
	if !q.Queued("s1") {
		t.Error("Queued = false with a message waiting")
	}
	if !q.Take("s1") {
		t.Error("Take returned nothing")
	}
	if q.Queued("s1") {
		t.Error("the queue still holds a message after Take")
	}
	// Another session is unaffected.
	if err := q.Enqueue("s2"); err != nil {
		t.Errorf("Enqueue(s2): %v", err)
	}
}

// --- the restart route ----------------------------------------------------------------------------------

// A session killed mid-turn comes back with its draft, its open question and its log, and resumes from the cut.
// This is the guarantee the whole log exists for; everything else in it is detail.
func TestASessionSurvivesARestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "sepia.db")
	var next int
	id := func() string { next++; return "id-" + itoa(next) }
	open := func() (*store.Store, *Engine) {
		db, err := store.Open(ctx, store.Options{Path: path, NewID: id})
		if err != nil {
			t.Fatalf("store.Open: %v", err)
		}
		if err := db.Migrate(ctx); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
		return db, New(Config{Store: db, NewID: id, ServerVersion: "test"})
	}

	db, engine := open()
	if err := db.SaveWork(ctx, domain.Work{ID: "w1", Medium: "book", Title: "The Lantern Keeper", Verified: true}); err != nil {
		t.Fatalf("SaveWork: %v", err)
	}
	sess := draftFor(t, engine, db, "w1")
	if _, err := engine.EditDraft(ctx, sess.ID, Edit{BaseVersion: 0, Set: patch(map[string]any{"rating": 16})}); err != nil {
		t.Fatalf("EditDraft: %v", err)
	}
	question, err := engine.AskRound(ctx, sess.ID, "question_form", map[string]any{"questions": []string{"why"}})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	head, err := db.HeadSequence(ctx, sess.ID)
	if err != nil {
		t.Fatalf("HeadSequence: %v", err)
	}
	_ = db.Close()

	// The process dies here. A new one opens the same file.
	db2, engine2 := open()
	defer func() { _ = db2.Close() }()
	hello, err := engine2.Greet(ctx, sess.ID, 0)
	if err != nil {
		t.Fatalf("Greet after restart: %v", err)
	}
	if hello.Draft == nil || hello.Draft.Entry.Rating != 16 {
		t.Fatalf("draft after restart = %+v", hello.Draft)
	}
	if len(hello.Open) != 1 || hello.Open[0].ID != question.ID {
		t.Fatalf("open questions after restart = %+v", hello.Open)
	}
	if hello.HeadSeq != head {
		t.Errorf("head = %d, want %d", hello.HeadSeq, head)
	}
	// Resuming from the middle replays the rest, in order, with no gap.
	replay, err := engine2.Greet(ctx, sess.ID, head-1)
	if err != nil {
		t.Fatalf("Greet from a cut: %v", err)
	}
	if len(replay.Events) != 1 || replay.Events[0].Seq != head {
		t.Errorf("replay = %+v", replay.Events)
	}
	// And the question can still be answered by the new process.
	if _, err := engine2.Answer(ctx, sess.ID, question.ID, map[string]any{"answers": []string{"because"}}, "m1"); err != nil {
		t.Fatalf("Answer after restart: %v", err)
	}
}

// itoa keeps the test's id source small.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// --- reopen: Edit on a saved card -----------------------------------------------------------------------

// saveOne makes a saved entry out of a fresh draft session on this engine and returns it.
func saveOne(t *testing.T, e *Engine, db *store.Store) domain.Entry {
	t.Helper()
	ctx := context.Background()
	sess := draftFor(t, e, db, "w1")
	draft, err := e.EditDraft(ctx, sess.ID, Edit{BaseVersion: 0, Set: patch(map[string]any{
		"rating": 15, "body": "First thoughts, which I want to change.",
	})})
	if err != nil {
		t.Fatalf("EditDraft: %v", err)
	}
	saved, _, _, err := e.Confirm(ctx, sess.ID, ConfirmRequest{BaseVersion: draft.Version})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	return saved
}

func TestReopenOpensARevisionOfASavedEntry(t *testing.T) {
	e, db := newEngine(t)
	ctx := context.Background()
	if err := db.SaveWork(ctx, domain.Work{ID: "w1", Medium: "book", Title: "The Lantern Keeper"}); err != nil {
		t.Fatal(err)
	}
	saved := saveOne(t, e, db)

	next, err := e.CreateEntry(ctx)
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	revision, err := e.Reopen(ctx, next.ID, saved.ID, "r1")
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if revision.Entry.ID == saved.ID || revision.Entry.Status != domain.StatusDraft {
		t.Fatalf("working copy = %+v, want a draft with its own id", revision.Entry)
	}
	if revision.Entry.Rating != saved.Rating || revision.Entry.Body != saved.Body || revision.Entry.WorkID != saved.WorkID {
		t.Errorf("working copy = %+v, want the saved fields", revision.Entry)
	}

	// The same client message again is a duplicate, not a second card.
	if _, duplicate := e.Reopen(ctx, next.ID, saved.ID, "r1"); !IsDuplicate(duplicate) {
		t.Errorf("retry = %v, want a duplicate", duplicate)
	}

	edited, err := e.EditDraft(ctx, next.ID, Edit{BaseVersion: revision.Version, Set: patch(map[string]any{
		"rating": 17,
	})})
	if err != nil {
		t.Fatalf("EditDraft: %v", err)
	}
	entry, _, _, err := e.Confirm(ctx, next.ID, ConfirmRequest{BaseVersion: edited.Version, Revises: saved.ID})
	if err != nil {
		t.Fatalf("Confirm revision: %v", err)
	}
	if entry.ID != saved.ID || entry.Rating != 17 {
		t.Errorf("entry = %+v, want the saved row with the new rating", entry)
	}
	revisions, err := db.Revisions(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Revisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].Snapshot.Rating != 15 {
		t.Errorf("revisions = %+v, want the first version kept", revisions)
	}
}

func TestReopenRefusesWhatItCannotOpen(t *testing.T) {
	e, db, sess := draftSession(t)
	ctx := context.Background()
	if _, err := e.Reopen(ctx, sess.ID, "e-missing", "r1"); err == nil {
		t.Fatal("an unknown entry was opened")
	} else if !IsNotFound(err) {
		t.Errorf("unknown entry = %v, want not_found", err)
	}

	// The card in this session is a draft, not a saved entry: there is nothing to revise.
	draftID, err := db.Session(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := e.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Reopen(ctx, next.ID, draftID.EntryID, "r1"); err == nil {
		t.Fatal("a draft was reopened as a saved entry")
	} else {
		var refusal *Refusal
		if !errors.As(err, &refusal) || refusal.Code != CodeInvalidValue {
			t.Errorf("draft entry = %v, want invalid_value", err)
		}
	}
}

func TestReopenRefusesAClosedSessionAndADeletedEntry(t *testing.T) {
	e, db := newEngine(t)
	ctx := context.Background()
	if err := db.SaveWork(ctx, domain.Work{ID: "w1", Medium: "book", Title: "The Lantern Keeper"}); err != nil {
		t.Fatal(err)
	}
	saved := saveOne(t, e, db)

	closed, err := e.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := e.Close(ctx, closed.ID, domain.SessionDiscarded); closeErr != nil {
		t.Fatal(closeErr)
	}
	if _, reopenErr := e.Reopen(ctx, closed.ID, saved.ID, "r1"); !errors.Is(reopenErr, ErrClosed) {
		t.Errorf("closed session = %v, want ErrClosed", reopenErr)
	}

	if deleteErr := db.SoftDeleteEntry(ctx, saved.ID); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	open, err := e.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Reopen(ctx, open.ID, saved.ID, "r1"); err == nil {
		t.Fatal("a deleted entry was reopened")
	}
}

// The revision target is the session's, not the process's. A restart loses the hub's memory, and confirm has to
// still write onto the saved entry rather than adding the working copy to the library as a second review.
func TestARevisionSurvivesARestartBecauseTheSessionKnowsIt(t *testing.T) {
	e, db := newEngine(t)
	ctx := context.Background()
	if err := db.SaveWork(ctx, domain.Work{ID: "w1", Medium: "book", Title: "The Lantern Keeper"}); err != nil {
		t.Fatal(err)
	}
	saved := saveOne(t, e, db)

	next, err := e.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := e.Reopen(ctx, next.ID, saved.ID, "r1")
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	edited, err := e.EditDraft(ctx, next.ID, Edit{BaseVersion: revision.Version, Set: patch(map[string]any{
		"rating": 19,
	})})
	if err != nil {
		t.Fatalf("EditDraft: %v", err)
	}

	// No Revises: the hub that remembered it is gone, which is what a restart leaves behind.
	entry, _, _, err := e.Confirm(ctx, next.ID, ConfirmRequest{BaseVersion: edited.Version})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if entry.ID != saved.ID {
		t.Errorf("entry id = %q, want the saved entry %q", entry.ID, saved.ID)
	}
	if entry.Rating != 19 {
		t.Errorf("rating = %d, want 19", entry.Rating)
	}
	revisions, err := db.Revisions(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Revisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].Snapshot.Rating != 15 {
		t.Errorf("revisions = %+v, want the first version kept", revisions)
	}
}

// A revision of an extracted review has no messages of its own. Its body is the one she proved when she first
// saved, so confirm checks it against the saved body instead of refusing a card she cannot confirm at all.
func TestARevisionOfAnExtractedBodyCanBeConfirmed(t *testing.T) {
	e, db := newEngine(t)
	ctx := context.Background()
	if err := db.SaveWork(ctx, domain.Work{ID: "w1", Medium: "book", Title: "The Lantern Keeper"}); err != nil {
		t.Fatal(err)
	}
	source := "I finished the lantern keeper last night and the ending landed but it was too long"
	first := draftFor(t, e, db, "w1")
	if _, err := e.AppendUserMessage(ctx, first.ID, source, "m1"); err != nil {
		t.Fatal(err)
	}
	// The verdict sits outside the body, which is normal: it is a quote from the same message.
	if _, err := e.SetExtractedBody(ctx, first.ID, 0, source,
		[]string{"but it was too long"}, "the ending landed", "m2"); err != nil {
		t.Fatalf("SetExtractedBody: %v", err)
	}
	draft, err := e.Draft(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	draft, err = e.EditDraft(ctx, first.ID, Edit{BaseVersion: draft.Version, Set: patch(map[string]any{"rating": 18})})
	if err != nil {
		t.Fatalf("EditDraft: %v", err)
	}
	saved, _, _, err := e.Confirm(ctx, first.ID, ConfirmRequest{
		BaseVersion: draft.Version, SourceText: source,
	})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if saved.BodyOrigin != domain.BodyExtracted {
		t.Fatalf("saved body origin = %s, want extracted", saved.BodyOrigin)
	}

	next, err := e.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := e.Reopen(ctx, next.ID, saved.ID, "r1")
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	// No source text: the revision session has no messages, and the body came from the saved entry.
	if target, targetErr := db.RevisionTarget(ctx, next.ID); targetErr != nil || target != saved.ID {
		t.Fatalf("target = %q, %v, want %q", target, targetErr, saved.ID)
	}
	if revision.Entry.BodyOrigin != domain.BodyExtracted {
		t.Fatalf("revision body origin = %s, want extracted", revision.Entry.BodyOrigin)
	}
	// The card the screen draws has to agree with the commit: Save is enabled for a card she never touched.
	card, err := e.Card(ctx, next.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if !card.Confirmable || len(card.Missing) != 0 {
		t.Fatalf("card = confirmable %v, missing %v, want a saveable card", card.Confirmable, card.Missing)
	}
	entry, _, _, err := e.Confirm(ctx, next.ID, ConfirmRequest{BaseVersion: revision.Version})
	if err != nil {
		t.Fatalf("Confirm revision: %v", err)
	}
	if entry.ID != saved.ID || entry.Body != saved.Body {
		t.Errorf("entry = %+v, want the saved entry and body", entry)
	}
}

// Log again is a second entry on the same work: a fresh card that asks for the rating and the words, rather than a
// copy of the one she already wrote.
func TestReopenAsNewLogsASecondEntryOnTheSameWork(t *testing.T) {
	e, db := newEngine(t)
	ctx := context.Background()
	if err := db.SaveWork(ctx, domain.Work{ID: "w1", Medium: "book", Title: "The Lantern Keeper"}); err != nil {
		t.Fatal(err)
	}
	saved := saveOne(t, e, db)

	next, err := e.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := e.ReopenAs(ctx, next.ID, ReopenRequest{EntryID: saved.ID, ClientMsgID: "r1", Mode: ReopenNew})
	if err != nil {
		t.Fatalf("ReopenAs new: %v", err)
	}
	if fresh.Entry.ID == saved.ID || fresh.Entry.Body != "" || fresh.Entry.Rating != 0 {
		t.Fatalf("a fresh card = %+v, want an empty one", fresh.Entry)
	}
	if fresh.Entry.WorkID != saved.WorkID {
		t.Fatalf("work = %q, want the same work", fresh.Entry.WorkID)
	}

	filled, err := e.EditDraft(ctx, next.ID, Edit{BaseVersion: fresh.Version, Set: patch(map[string]any{
		"rating": 19, "body": "Second time through, and it holds up.", "finished_on": "2026-11-01",
	})})
	if err != nil {
		t.Fatalf("EditDraft: %v", err)
	}
	// No revision target: this is a new entry, so confirming adds a row instead of rewriting the saved one.
	second, _, _, err := e.Confirm(ctx, next.ID, ConfirmRequest{BaseVersion: filled.Version})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if second.ID == saved.ID {
		t.Fatal("Log again rewrote the saved entry instead of adding one")
	}
	onWork, err := db.EntriesOnWork(ctx, "w1")
	if err != nil {
		t.Fatal(err)
	}
	if len(onWork) != 2 {
		t.Fatalf("entries on the work = %d, want the saved one and the new one", len(onWork))
	}
	if revisions, revErr := db.Revisions(ctx, saved.ID); revErr != nil || len(revisions) != 0 {
		t.Fatalf("the first entry gained revisions: %+v %v", revisions, revErr)
	}
}

// Restore as draft opens an older version. Confirming makes it the newest version and keeps the one it replaced, so
// nothing is ever lost by going back.
func TestReopenAsRevisionRestoresAnOlderVersion(t *testing.T) {
	e, db := newEngine(t)
	ctx := context.Background()
	if err := db.SaveWork(ctx, domain.Work{ID: "w1", Medium: "book", Title: "The Lantern Keeper"}); err != nil {
		t.Fatal(err)
	}
	saved := saveOne(t, e, db)

	// Two edits, so the history is: v1 the first save, v2 what the first edit replaced, v3 what the second replaced.
	for _, rating := range []int{17, 19} {
		next, err := e.CreateEntry(ctx)
		if err != nil {
			t.Fatal(err)
		}
		revision, err := e.Reopen(ctx, next.ID, saved.ID, "edit-"+strconv.Itoa(rating))
		if err != nil {
			t.Fatalf("Reopen: %v", err)
		}
		edited, err := e.EditDraft(ctx, next.ID, Edit{BaseVersion: revision.Version, Set: patch(map[string]any{
			"rating": rating,
		})})
		if err != nil {
			t.Fatalf("EditDraft: %v", err)
		}
		if _, _, _, err = e.Confirm(ctx, next.ID, ConfirmRequest{BaseVersion: edited.Version, Revises: saved.ID}); err != nil {
			t.Fatalf("Confirm: %v", err)
		}
	}
	versions, err := db.Revisions(ctx, saved.ID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("revisions = %+v %v, want two", versions, err)
	}
	oldest := versions[len(versions)-1]

	next, err := e.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := e.ReopenAs(ctx, next.ID, ReopenRequest{EntryID: saved.ID, ClientMsgID: "restore", Revision: oldest.Number})
	if err != nil {
		t.Fatalf("ReopenAs revision: %v", err)
	}
	if restored.Entry.Body != oldest.Snapshot.Body || restored.Entry.Rating != oldest.Snapshot.Rating {
		t.Fatalf("restored draft = %+v, want version %d", restored.Entry, oldest.Number)
	}
	if _, _, _, err = e.Confirm(ctx, next.ID, ConfirmRequest{BaseVersion: restored.Version, Revises: saved.ID}); err != nil {
		t.Fatalf("Confirm restore: %v", err)
	}
	current, err := db.Entry(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Rating != oldest.Snapshot.Rating {
		t.Fatalf("the entry = %d stars, want the restored version's", current.Rating)
	}
	// The version it replaced is kept, so the history still holds every step.
	after, err := db.Revisions(ctx, saved.ID)
	if err != nil || len(after) != 3 {
		t.Fatalf("revisions after restore = %d %v, want three", len(after), err)
	}
}

func TestReopenAsRefusesWhatItCannotOpen(t *testing.T) {
	e, db := newEngine(t)
	ctx := context.Background()
	if err := db.SaveWork(ctx, domain.Work{ID: "w1", Medium: "book", Title: "The Lantern Keeper"}); err != nil {
		t.Fatal(err)
	}
	saved := saveOne(t, e, db)

	cases := map[string]ReopenRequest{
		"a version that is not there": {EntryID: saved.ID, Revision: 9},
		"a mode it does not know":     {EntryID: saved.ID, Mode: "delete"},
		"new and a version at once":   {EntryID: saved.ID, Mode: ReopenNew, Revision: 1},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			next, err := e.CreateEntry(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = e.ReopenAs(ctx, next.ID, in)
			var refusal *Refusal
			if !errors.As(err, &refusal) || refusal.Code != CodeInvalidValue {
				t.Fatalf("err = %v, want invalid_value", err)
			}
		})
	}
}
