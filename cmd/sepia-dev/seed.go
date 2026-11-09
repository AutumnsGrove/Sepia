package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/session"
	"github.com/AutumnsGrove/sepia/internal/store"
)

// The invented library the reading screens are looked at with, on and off the end-to-end suite.
//
// Everything here goes through the real engine: a draft, an edit, a confirm, the same calls the app makes. Nothing is
// written straight into a table, and nothing here is a review of a real work or a real opinion of Autumn's
// (docs/BUILD.md invariant 10). The seed only fills an empty database, because a dev or e2e stack that reused its
// data directory must not end up with two of everything.

// seedIDs names the rows the screen tests point at, so a test never has to guess which library row is which.
type seedIDs struct {
	// LibraryCount is how many saved entries the library holds after seeding, which the second run must not change.
	LibraryCount int
	// LanternWorkID carries two entries, which is the "also on this work" timeline on an entry page.
	LanternWorkID string
	// RevisedEntryID has three versions, which is the versions screen.
	RevisedEntryID string
	// DeletedEntryID is in the Deleted list, and restores.
	DeletedEntryID string
}

// seededEntry is one invented review. Field names follow the card, so a reader can check it against the mockups.
type seededEntry struct {
	Title      string
	Medium     string
	Year       int
	Creator    string
	CreatorOf  string
	Format     string
	FinishedOn string
	Rating     int
	Favourite  bool
	Body       string
	// SameWorkAs makes this a second entry on an existing seeded title, which is a revisit rather than a new work.
	SameWorkAs string
	Answers    []domain.Answer
	// Edits are the revisions the entry gets after its first save, oldest first, so the versions screen has history.
	Edits []seededEdit
	// Delete moves the entry to the Deleted list.
	Delete bool
}

type seededEdit struct {
	Rating int
	Body   string
}

// seedEntries is the library in the order it is written, oldest first, so the newest is last and the screen's order
// is not simply the write order.
func seedEntries() []seededEntry {
	return []seededEntry{
		{
			Title: "A Quiet Cartography", Medium: "book", Year: 2019, Creator: "N. J. Alder", CreatorOf: "author",
			Format: "paperback", FinishedOn: "2024-03-12", Rating: 16,
			Body: "Maps as a way of remembering people rather than places. The middle lost me for a while, but the way it came back together was worth it.",
		},
		{
			Title: "The Lantern Keeper", Medium: "book", Year: 2024, Creator: "R. O. Ingram", CreatorOf: "author",
			Format: "hardback", FinishedOn: "2024-01-15", Rating: 16,
			Body: "Read it in two sittings on a cold weekend. The lighthouse is barely a metaphor by the end, which I liked.",
		},
		{
			Title: "Night Signal", Medium: "album", Year: 2025, Creator: "Loam", CreatorOf: "artist",
			Format: "streaming", FinishedOn: "2026-06-02", Rating: 16,
			Body: "Ten tracks and not one of them long. The closing track is the whole record in five minutes.",
		},
		{
			Title: "The Longest Winter", Medium: "film", Year: 2023, Creator: "I. Sable", CreatorOf: "director",
			Format: "streaming", FinishedOn: "2026-07-14", Rating: 14,
			Delete: true,
			Body:   "Beautifully shot and far too long. Logged this before I had finished it, which was my mistake.",
		},
		{
			Title: "Grey Harbour", Medium: "tv", Year: 2026, Creator: "M. Ferrier", CreatorOf: "creator",
			Format: "streaming", FinishedOn: "2026-09-14", Rating: 18,
			Body: "Season one is a slow burn that earns its ending. Two performances carry the last three episodes entirely.",
		},
		{
			Title: "The Salt Road", Medium: "book", Year: 2021, Creator: "T. Okonjo", CreatorOf: "author",
			Format: "paperback", FinishedOn: "2026-09-28", Rating: 19,
			Body: "Small cast, no set to speak of, and it still filled a whole afternoon. I kept stopping to reread sentences.",
		},
		{
			Title: "The Copper Room", Medium: "theatre", Year: 2026, Creator: "F. Halloway", CreatorOf: "director",
			Format: "live", FinishedOn: "2026-10-18", Rating: 20, Favourite: true,
			Body: "Standing at the back of the stalls and I still forgot where I was. The final monologue is the best thing I have seen this year.",
		},
		{
			Title: "Hollow Deep", Medium: "game", Year: 2026, Creator: "Ninefold", CreatorOf: "studio",
			Format: "digital", FinishedOn: "2026-10-12", Rating: 17,
			Body: "The boss fights are the best kind of unfair, and the map is a joy to move through. The story is mostly noise.",
		},
		{
			Title: "Bramble Hollow", Medium: "film", Year: 2025, Creator: "J. Mire", CreatorOf: "director",
			Format: "cinema", FinishedOn: "2026-10-27", Rating: 19, Favourite: true,
			Body: "loved it honestly, easy 4.5, the sound design carried the whole thing and the middle dragged a little",
			Edits: []seededEdit{
				{Rating: 18, Body: "loved it honestly, easy 4.5, the sound design carried the whole thing and the middle dragged a little"},
				{
					Rating: 19,
					Body:   "loved it honestly, easy 4.5, the sound design carried the whole thing and the middle dragged a little, still thinking about the last shot",
				},
			},
			Answers: []domain.Answer{
				{
					QuestionID: "visuals", Category: "visuals", Prompt: "Visuals", Type: domain.AnswerTags,
					Values: []string{"striking images", "beautifully shot"},
				},
				{
					QuestionID: "sound", Category: "sound", Prompt: "Sound and music", Type: domain.AnswerTags,
					Values: []string{"sound design carried it"},
				},
				{
					QuestionID: "ending_landed", Prompt: "Ending", Type: domain.AnswerChoice,
					Values: []string{"it landed"},
				},
			},
		},
		{
			Title: "The Lantern Keeper", Medium: "book", Year: 2024, Creator: "R. O. Ingram", CreatorOf: "author",
			Format: "audiobook", FinishedOn: "2026-10-30", Rating: 18,
			SameWorkAs: "The Lantern Keeper",
			Body:       "The narrator makes the quiet parts work. Second time through I noticed how much of it is about waiting.",
			Answers: []domain.Answer{
				{
					QuestionID: "characters", Category: "characters", Prompt: "Characters", Type: domain.AnswerTags,
					Values: []string{"memorable", "multilayered"},
				},
				{
					QuestionID: "writing", Category: "writing", Prompt: "Writing style", Type: domain.AnswerTags,
					Values: []string{"beautiful prose"},
				},
			},
		},
		{
			Title: "The Salt Road", Medium: "book", Year: 2021, Creator: "T. Okonjo", CreatorOf: "author",
			Format: "paperback", FinishedOn: "", Rating: 19, SameWorkAs: "The Salt Road",
			Body: "Reread it and it holds up completely. No date on this one because I did not write down when I finished it.",
		},
	}
}

// seededDrafts are the two cards left open, each waiting for a different thing, which is what the Drafts list has to
// tell apart.
type seededDraft struct {
	Title string
	Work  string
	// Rating of zero leaves the card waiting for one; a rating with no body leaves it waiting for words.
	Rating int
}

func seedLibrary(ctx context.Context, db *store.Store, engine *session.Engine, log *slog.Logger) (seedIDs, error) {
	if !emptyLibrary(ctx, db) {
		library, _, err := db.LibraryPage(ctx, domain.LibraryQuery{}, domain.Cursor{}, 100)
		if err != nil {
			return seedIDs{}, err
		}
		log.Info("the invented library is already here", "saved", len(library))
		return seedIDs{LibraryCount: len(library), LanternWorkID: seedWorkID("The Lantern Keeper")}, nil
	}
	works := map[string]string{}
	ids := seedIDs{}
	entries := seedEntries()

	for _, seeded := range entries {
		workID := works[seeded.Title]
		if seeded.SameWorkAs != "" {
			workID = works[seeded.SameWorkAs]
		}
		if workID == "" {
			work, err := seedWork(ctx, db, seeded)
			if err != nil {
				return seedIDs{}, err
			}
			workID = work.ID
			works[seeded.Title] = workID
		}
		entry, err := seedSavedEntry(ctx, engine, seeded, workID)
		if err != nil {
			return seedIDs{}, fmt.Errorf("seed %q: %w", seeded.Title, err)
		}
		if len(seeded.Answers) > 0 {
			if err := db.AddAnswers(ctx, entry.ID, seeded.Answers); err != nil {
				return seedIDs{}, err
			}
		}
		if len(seeded.Edits) > 0 {
			if err := seedEdits(ctx, engine, entry, seeded.Edits); err != nil {
				return seedIDs{}, fmt.Errorf("seed edits for %q: %w", seeded.Title, err)
			}
			ids.RevisedEntryID = entry.ID
		}
		if seeded.Delete {
			if err := db.SoftDeleteEntry(ctx, entry.ID); err != nil {
				return seedIDs{}, err
			}
			ids.DeletedEntryID = entry.ID
		}
		if seeded.Title == "The Lantern Keeper" && seeded.Medium == "book" {
			ids.LanternWorkID = workID
		}
	}

	for _, draft := range seedDrafts() {
		workID := works[draft.Work]
		if workID == "" {
			return seedIDs{}, fmt.Errorf("seed draft %q: no such seeded work", draft.Title)
		}
		sess, err := engine.CreateEntry(ctx)
		if err != nil {
			return seedIDs{}, err
		}
		card, err := engine.StartDraft(ctx, sess.ID, domain.Entry{WorkID: workID})
		if err != nil {
			return seedIDs{}, err
		}
		if draft.Rating > 0 {
			if _, err = engine.EditDraft(ctx, sess.ID, session.Edit{BaseVersion: card.Version, Set: map[string]json.RawMessage{
				"rating": json.RawMessage(fmt.Sprintf("%d", draft.Rating)),
			}}); err != nil {
				return seedIDs{}, err
			}
		}
	}

	library, _, err := db.LibraryPage(ctx, domain.LibraryQuery{}, domain.Cursor{}, 100)
	if err != nil {
		return seedIDs{}, err
	}
	ids.LibraryCount = len(library)
	log.Info("seeded the invented library", "saved", len(library), "drafts", len(seedDrafts()))
	return ids, nil
}

// seedDrafts is the two open cards. They name seeded works by title, so the ids come from the same map.
func seedDrafts() []seededDraft {
	return []seededDraft{
		{Title: "Hollow Deep", Work: "Hollow Deep"},
		{Title: "Grey Harbour", Work: "Grey Harbour", Rating: 18},
	}
}

func seedWork(ctx context.Context, db *store.Store, seeded seededEntry) (domain.Work, error) {
	id := seedWorkID(seeded.Title)
	work := domain.Work{
		ID: id, Medium: seeded.Medium, Title: seeded.Title, Year: seeded.Year, Verified: true,
		Creators:        []domain.Creator{{Role: seeded.CreatorOf, Name: seeded.Creator}},
		PrimaryProvider: "openlibrary", PrimaryID: id,
	}
	if err := db.SaveWork(ctx, work); err != nil {
		return domain.Work{}, err
	}
	return work, nil
}

// seedWorkID is stable across runs, so a second seed recognises its own rows rather than adding a second copy.
func seedWorkID(title string) string {
	slug := make([]rune, 0, len(title))
	for _, r := range title {
		if r == ' ' {
			slug = append(slug, '-')
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		slug = append(slug, r)
	}
	return "seed:" + string(slug)
}

// seedSavedEntry writes one review the way the app does: a draft, the fields on it, then the confirm tap.
func seedSavedEntry(ctx context.Context, engine *session.Engine, seeded seededEntry, workID string) (domain.Entry, error) {
	sess, err := engine.CreateEntry(ctx)
	if err != nil {
		return domain.Entry{}, err
	}
	card, err := engine.StartDraft(ctx, sess.ID, domain.Entry{WorkID: workID})
	if err != nil {
		return domain.Entry{}, err
	}
	set := map[string]json.RawMessage{
		"rating":      json.RawMessage(fmt.Sprintf("%d", seeded.Rating)),
		"body":        json.RawMessage(quote(seeded.Body)),
		"format":      json.RawMessage(quote(seeded.Format)),
		"favourite":   json.RawMessage(fmt.Sprintf("%t", seeded.Favourite)),
		"finished_on": json.RawMessage(quote(seeded.FinishedOn)),
	}
	edited, err := engine.EditDraft(ctx, sess.ID, session.Edit{BaseVersion: card.Version, Set: set})
	if err != nil {
		return domain.Entry{}, err
	}
	entry, _, _, err := engine.Confirm(ctx, sess.ID, session.ConfirmRequest{BaseVersion: edited.Version})
	return entry, err
}

// seedEdits replays the confirmed edits that give an entry its version history.
func seedEdits(ctx context.Context, engine *session.Engine, saved domain.Entry, edits []seededEdit) error {
	for i, edit := range edits {
		sess, err := engine.CreateEntry(ctx)
		if err != nil {
			return err
		}
		revision, err := engine.Reopen(ctx, sess.ID, saved.ID, fmt.Sprintf("seed-edit-%s-%d", saved.ID, i))
		if err != nil {
			return err
		}
		edited, err := engine.EditDraft(ctx, sess.ID, session.Edit{BaseVersion: revision.Version, Set: map[string]json.RawMessage{
			"rating": json.RawMessage(fmt.Sprintf("%d", edit.Rating)),
			"body":   json.RawMessage(quote(edit.Body)),
		}})
		if err != nil {
			return err
		}
		if _, _, _, err = engine.Confirm(ctx, sess.ID, session.ConfirmRequest{BaseVersion: edited.Version, Revises: saved.ID}); err != nil {
			return err
		}
	}
	return nil
}

// emptyLibrary reports whether this database has nothing seeded in it yet. A dev stack pointed at a data directory it
// used before must keep what is there.
func emptyLibrary(ctx context.Context, db *store.Store) bool {
	_, err := db.Work(ctx, seedWorkID(seedEntries()[0].Title))
	return err != nil
}

func quote(value string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(raw)
}

// seedNote is the line the dev stack prints, so the invented library is never mistaken for hers.
func seedNote(ids seedIDs) string {
	return fmt.Sprintf("%d invented reviews, %d open drafts, one deleted entry; titles and reviews are made up",
		ids.LibraryCount, len(seedDrafts()))
}
