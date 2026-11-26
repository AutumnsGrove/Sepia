package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/AutumnsGrove/nova/tool"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/session"
)

// The library chat. A general session is a conversation about the library rather than a card, so it gets its own
// system prompt and its own two tools: one read and one proposal. There is no delete tool and no tool that writes an
// entry directly, which is what makes "the chat can never delete" true of the code rather than of the prompt
// (docs/slices/S16-search-and-chat.md, S16.4).

// newChatTools is the whole chat vocabulary. It is built beside the entry tools rather than filtered from them, so a
// tool added for logging a review cannot become reachable from a chat by accident.
func (a *Agent) newChatTools() *tool.Registry {
	reg := tool.NewRegistry()
	reg.MustRegister(tool.New(tool.Info{
		Name: "search_library", Description: toolDescription("search_library"),
		Parameters: object(map[string]any{
			"query":  stringProp("Words to find in a title, a saved review or an answer she gave."),
			"medium": enumProp(domain.Mediums(), "Only works of this medium."),
			"from":   map[string]any{"type": "string", "description": "Only entries finished on or after this day, YYYY-MM-DD."},
			"to":     map[string]any{"type": "string", "description": "Only entries finished on or before this day, YYYY-MM-DD."},
		}, []string{"query"}),
		Parallel: true, ReadOnly: true,
	}, searchLibrary))

	reg.MustRegister(tool.New(tool.Info{
		Name: "edit_entry", Description: toolDescription("edit_entry"),
		Parameters: object(map[string]any{
			"entry_id": stringProp("The id of the saved entry to change, as a search returned it."),
			"rating": map[string]any{
				"type": "number", "minimum": 0.25, "maximum": 5, "multipleOf": 0.25,
				"description": "The new rating in stars, a quarter at a time.",
			},
			"completion": map[string]any{
				"type": "string", "enum": []string{"finished", "abandoned", "want_to"},
				"description": "The shelf: finished, abandoned (started and stopped) or want_to (not read yet).",
			},
			"favourite":   map[string]any{"type": "boolean", "description": "Whether it is a favourite."},
			"format":      stringProp("How she experienced it, for example paperback or cinema."),
			"finished_on": map[string]any{"type": "string", "description": "The day, YYYY-MM-DD."},
		}, []string{"entry_id"}),
		ReadOnly: false,
	}, editEntry))
	return reg
}

// editArgs is what the model may send. A field it leaves out is a field it is not changing.
type editArgs struct {
	EntryID    string   `json:"entry_id"`
	Rating     *float64 `json:"rating,omitempty"`
	Completion string   `json:"completion,omitempty"`
	Favourite  *bool    `json:"favourite,omitempty"`
	Format     string   `json:"format,omitempty"`
	FinishedOn string   `json:"finished_on,omitempty"`
}

// editEntry proposes one change to one saved entry and asks her to confirm it.
//
// The write happens here rather than in a later turn, but only after the interaction comes back as confirm, so the
// rule that holds on the card holds in the chat: nothing is written without her tap. The revision it replaces is kept,
// because the write is the same store call a typed edit uses (docs/ENTRY_CARD.md 7).
func editEntry(ctx context.Context, raw json.RawMessage, tctx *tool.Context) (tool.Result, error) {
	state, ok := tctx.Data.(*RunState)
	if !ok {
		return tool.Errorf("the review state is not wired"), nil
	}
	var args editArgs
	if err := decodeArgs(raw, &args); err != nil {
		return tool.Errorf("bad arguments: %v", err), nil
	}
	if strings.TrimSpace(args.EntryID) == "" {
		return tool.Errorf("say which entry, using the id a search returned"), nil
	}
	if !state.responderAvailable() {
		return tool.Errorf("there is nobody to confirm a change"), nil
	}

	entry, err := state.Agent.store.Entry(ctx, args.EntryID)
	if err != nil {
		return tool.Errorf("there is no entry with that id"), nil
	}
	if entry.Status != domain.StatusSaved {
		return tool.Errorf("that is a draft, not a saved entry"), nil
	}
	edits, err := args.edits()
	if err != nil {
		return tool.Errorf("%v", err), nil
	}
	proposed, err := domain.ApplyEdits(entry, edits)
	if err != nil {
		return tool.Errorf("%s", err.Error()), nil
	}
	changes := domain.ChangesOf(entry, proposed)
	if len(changes) == 0 {
		// She asked for the value it already has. Saying so is the honest answer; a card that changes nothing is not.
		return tool.Errorf("the entry already has %s", args.described(entry)), nil
	}

	title := entry.ID
	if work, workErr := state.Agent.store.Work(ctx, entry.WorkID); workErr == nil && work.Title != "" {
		title = work.Title
	}
	baseVersion := entry.UpdatedAt
	resolution, askErr := state.askAny(ctx, "edit_entry", map[string]any{
		"entry_id":     entry.ID,
		"title":        title,
		"base_version": baseVersion,
		"summary":      args.described(entry),
		"changes":      changes,
	})
	if askErr != nil {
		return tool.Errorf("the change could not be shown: %v", askErr), nil
	}
	if choice, _ := decodeChoice(resolution); choice != "confirm" {
		return resultWithDetails(toolDetails{Summary: "Change cancelled"},
			"She cancelled. Nothing was changed."), nil
	}

	// The entry may have moved between the proposal and the tap, on this screen or another one. The stored version is
	// the one the diff was drawn from, so a mismatch refuses rather than overwriting a change she has not seen.
	current, err := state.Agent.store.Entry(ctx, entry.ID)
	if err != nil {
		return tool.Errorf("the entry could not be read again: %v", err), nil
	}
	if current.UpdatedAt != baseVersion {
		return tool.Errorf("%s", session.CodeStaleVersion), nil
	}
	revision, err := state.Agent.store.SaveEntry(ctx, proposed)
	if err != nil {
		return tool.Errorf("the change could not be saved: %v", err), nil
	}
	state.EditedEntry = entry.ID
	summary := changeSummary(changes)
	if revision > 0 {
		summary += " (version " + strconv.Itoa(revision+1) + ")"
	}
	return resultWithDetails(toolDetails{Summary: summary},
		"It is done: %s. The previous version is kept.", summary), nil
}

// edits turns the arguments into the domain's shape, treating an empty string as "not changing that field" because a
// model that omits nothing still should not be able to blank a value by accident.
func (a editArgs) edits() (domain.Edits, error) {
	out := domain.Edits{}
	if a.Rating != nil {
		quarters := domain.RatingFromStars(*a.Rating)
		if quarters == 0 {
			return domain.Edits{}, errors.New("that is not a rating between a quarter and five stars")
		}
		out.Rating = &quarters
	}
	if a.Completion != "" {
		completion, known := domain.ParseCompletion(a.Completion)
		if !known {
			return domain.Edits{}, errors.New("that is not a shelf Sepia knows")
		}
		out.Completion = &completion
	}
	if a.Favourite != nil {
		out.Favourite = a.Favourite
	}
	if a.Format != "" {
		format := a.Format
		out.Format = &format
	}
	if a.FinishedOn != "" {
		day := a.FinishedOn
		out.FinishedOn = &day
	}
	if out.Empty() {
		return domain.Edits{}, errors.New("that change asks for nothing")
	}
	return out, nil
}

// described is what she already has in the fields this edit names, for the "nothing to change" answer and for the
// card's summary line.
func (a editArgs) described(e domain.Entry) string {
	parts := make([]string, 0, 3)
	if a.Rating != nil {
		parts = append(parts, domain.FormatRating(e.Rating)+" stars")
	}
	if a.Completion != "" {
		parts = append(parts, string(e.CompletionState()))
	}
	if a.Favourite != nil {
		if e.Favourite {
			parts = append(parts, "a favourite")
		} else {
			parts = append(parts, "not a favourite")
		}
	}
	if a.Format != "" {
		parts = append(parts, e.Format)
	}
	if a.FinishedOn != "" {
		parts = append(parts, e.FinishedOn)
	}
	if len(parts) == 0 {
		return "nothing this tool can change"
	}
	return strings.Join(parts, ", ")
}

// changeSummary names the change in the words the recents list and the activity line use: "Rating changed".
func changeSummary(changes []domain.EditChange) string {
	labels := make([]string, 0, len(changes))
	for _, change := range changes {
		labels = append(labels, change.Label)
	}
	return strings.Join(labels, " and ") + " changed"
}

// handleChat is one turn of a general session: her question, the chat's tools, and an answer. It has no pre-read, no
// parse and no card, so it does not run the entry pipeline; what it shares with an entry turn is the provider, the
// event stream, the ledgers and the deadline, which runModel already owns.
func (a *Agent) handleChat(ctx context.Context, in TurnInput) (Outcome, error) {
	state := &RunState{
		Agent: a, SessionID: in.SessionID, Text: in.Text, Chat: true,
		System: ChatSystemPrompt(), Tools: a.chatTools,
	}
	if a.provider == nil {
		if err := a.notice(ctx, in.SessionID, "warn", "model_off",
			"The model is not set up. Ask again once it is."); err != nil {
			return Outcome{}, err
		}
		return a.finish(ctx, state, "model_off")
	}
	runErr := a.runModel(ctx, state, "")
	if runErr != nil {
		code, text := "model_unavailable", "The model did not finish. Ask again when you are ready."
		switch {
		case errors.Is(runErr, context.Canceled):
			code, text = "stopped", "Stopped."
		case errors.Is(runErr, errBudget):
			code, text = "turn_budget", "This answer got long, so I stopped."
		case errors.Is(runErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
			code, text = "turn_timeout", "The model took longer than two minutes. Ask again."
		}
		if err := a.notice(ctx, in.SessionID, "warn", code, text); err != nil {
			return Outcome{}, err
		}
		a.log.Warn("chat turn failed", "session", in.SessionID, "reason", runErr.Error())
	}
	if state.isStopped() && runErr == nil {
		if err := a.notice(ctx, in.SessionID, "warn", "step_limit", "I stopped after several steps."); err != nil {
			return Outcome{}, err
		}
	}
	out, err := a.finish(ctx, state, state.ended())
	out.EditedEntry = state.EditedEntry
	return out, err
}
