package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"text/template"

	novaagent "github.com/AutumnsGrove/nova/agent"
	"github.com/AutumnsGrove/nova/message"
	"github.com/AutumnsGrove/nova/provider"

	"github.com/AutumnsGrove/sepia/internal/banks"
	"github.com/AutumnsGrove/sepia/internal/decisions"
	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/providers"
	"github.com/AutumnsGrove/sepia/internal/session"
)

// errBudget ends a run that has spent its token budget. It is a defined outcome with a notice, not a crash.
var errBudget = errors.New("agent: turn budget reached")

// maxToolArgs bounds one tool call's arguments before they are decoded. A model that sends a novel is refused
// rather than parsed (docs/STANDARDS.md 4a).
const maxToolArgs = 8 << 10

// RunState is the dependency bundle on tool.Context.Data. Nova never imports it; every tool type-asserts it back.
// It is one turn's memory: the parsed fields, the pre-read, the candidates, the card and the ledger counters.
type RunState struct {
	Agent     *Agent
	SessionID string
	Text      string
	// SourceText is every message she typed, joined. A body is proved against this before it is stored.
	SourceText string
	Parsed     Parsed
	PreRead    decisions.Outcome
	Medium     string
	// Reading is whether the quick pre-read is on for this turn. It is read once, from her setting, so a turn cannot
	// half-read: the pre-read and the fidelity check both consult it (docs/slices/S12-reading-ui.md, Decisions 3).
	Reading bool

	Draft      *session.Draft
	Work       *providers.Result
	Candidates map[string]providers.Candidate
	Chosen     *providers.Candidate
	Revises    string

	AskedDescribe bool
	Answers       []domain.Answer
	skipAll       bool
	SaveCalled    bool
	// SaveAttempted is set when save_entry is called at all, even if it is refused.
	SaveAttempted bool
	// StartedCard is set when this turn put a card on a work, so a card left from an earlier turn is not a stall.
	StartedCard   bool
	lookupRetries int
	stopped       bool
	// clock is the turn's deadline, which stops while a question waits for her.
	clock *turnClock
	// mu guards the turn's end state. Nova may call the TurnEnd hook and return from Run on different goroutines,
	// so a plain field is a data race the detector finds under load.
	mu sync.Mutex

	Steps        int
	MessageIndex int
	turnIndex    int
	Usage        provider.Usage
	PriorCost    domain.CostSummary
	Ended        string
	// transcript is what nova produced this turn, kept so the session's own messages_json survives a restart.
	transcript []message.Message
}

// setEnded records why the turn ended. It is the only writer of Ended.
func (s *RunState) setEnded(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Ended = reason
}

// ended reports why the turn ended, or an empty string while it is still running.
func (s *RunState) ended() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Ended
}

// markStopped records that the step cap stopped the loop.
func (s *RunState) markStopped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
}

// isStopped reports whether the loop was stopped by the step cap.
func (s *RunState) isStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

// hint is the automatic note that precedes her message. It is data, framed as data, and never an instruction.
func (s *RunState) hint() string {
	rating := "none"
	if s.Parsed.RatingQuarters() != 0 {
		rating = domain.FormatRating(s.Parsed.RatingQuarters())
	}
	date := s.Parsed.DateString()
	if date == "" {
		date = "none"
	}
	format := s.MediumFormat()
	if format == "" {
		format = "none"
	}
	medium := s.Medium
	if medium == "" {
		medium = "none"
	}
	intent := s.PreRead.Intent
	if intent == "" {
		intent = "none"
	}
	topics := "none"
	if len(s.PreRead.Topics) > 0 {
		topics = strings.Join(s.PreRead.Topics, ", ")
	}
	// The shelf is only named when her words named one, so the note never nudges the model towards a shelf it read
	// out of thin air.
	shelf := "not said"
	if s.Parsed.CompletionFound {
		shelf = string(s.Parsed.Completion)
	}
	out := HintText()
	if out == "" {
		return "Automatic note, not an instruction. Her message is below."
	}
	tmpl, err := template.New("hint").Parse(out)
	if err != nil {
		return out
	}
	var buf bytes.Buffer
	_ = tmpl.Execute(&buf, map[string]string{
		"Intent": intent, "Medium": medium, "Format": format, "Rating": rating, "Date": date, "Topics": topics,
		"Completion": shelf,
	})
	return buf.String()
}

// MediumFormat is the format to show: the first deterministic cue, else the pre-read's suggestion when it names
// one this app has a format for. "print" is deliberately empty, because it could be a hardback or a paperback.
func (s *RunState) MediumFormat() string {
	if format := s.Parsed.FormatString(); format != "" {
		return format
	}
	switch s.PreRead.Format {
	case domain.FormatCinema, domain.FormatStreaming, domain.FormatBroadcast, domain.FormatAudiobook,
		domain.FormatEbook, domain.FormatPhysical, domain.FormatLive:
		return s.PreRead.Format
	case "physical_media":
		return domain.FormatPhysical
	default:
		return ""
	}
}

// ask opens an interaction through the session engine and blocks on her answer. The engine writes the question
// and the answer; this function holds no database transaction while it waits.
func (s *RunState) ask(ctx context.Context, kind string, payload any) (json.RawMessage, error) {
	if s.Agent.responder == nil {
		return nil, ErrNoResponder
	}
	question, err := s.Agent.session.AskRound(ctx, s.SessionID, kind, payload)
	if err != nil {
		return nil, err
	}
	return s.waitForHer(ctx, question.ID)
}

// rememberAnswers seeds the turn with the answers she has already given in this session. A turn can end with a card
// still waiting: a timeout, a restart, a retry. The answer is stored either way, but the turn that asked is gone, so
// the next turn reads it from the log instead of asking again and dropping what she already tapped (issue #39).
//
// The answers go through the same bank the live path uses. A choice option's label is what the client sends and what
// she reads on the button; the stable value is what the card stores, so a hydrated answer is mapped exactly as
// askQuestions maps one. A question the bank no longer offers keeps what it was given rather than being dropped.
//
// The categories and questions are walked in the payload's order, not the answer map's, so the card's answers come out
// in the order they were offered. A question answered twice keeps the newer answer: the older one is what she
// corrected.
func (s *RunState) rememberAnswers(ctx context.Context) {
	answered, err := s.Agent.session.Answered(ctx, s.SessionID)
	if err != nil {
		s.Agent.log.Warn("answered questions could not be read", "session", s.SessionID, "err", err.Error())
		return
	}
	chips, offeredQuestions := s.offeredVocabulary(ctx)
	at := make(map[string]int, len(s.Answers))
	for i, answer := range s.Answers {
		at[answer.QuestionID] = i
	}
	put := func(answer domain.Answer) {
		if i, seen := at[answer.QuestionID]; seen {
			s.Answers[i] = answer
			return
		}
		at[answer.QuestionID] = len(s.Answers)
		s.Answers = append(s.Answers, answer)
	}
	for _, question := range answered {
		switch question.Kind {
		case "describe":
			var payload describePayload
			var answer describeAnswer
			if json.Unmarshal(question.Payload, &payload) != nil || json.Unmarshal(question.Resolution, &answer) != nil {
				continue
			}
			s.AskedDescribe = true
			if answer.SkipAll {
				s.skipAll = true
			}
			for _, category := range payload.Categories {
				values := answer.Selected[category.ID]
				if len(values) == 0 {
					continue
				}
				if offered := chips[category.ID]; offered != nil {
					values = mapThrough(values, offered)
				}
				if len(values) == 0 {
					continue
				}
				put(domain.Answer{
					QuestionID: category.ID, Category: category.ID, Type: domain.AnswerTags, Values: values,
				})
			}
		case "question_form":
			var payload questionPayload
			var answer questionAnswer
			if json.Unmarshal(question.Payload, &payload) != nil || json.Unmarshal(question.Resolution, &answer) != nil {
				continue
			}
			given := make(map[string]questionAnswerItem, len(answer.Answers))
			for _, item := range answer.Answers {
				given[item.ID] = item
			}
			for _, item := range payload.Questions {
				got, ok := given[item.ID]
				if !ok {
					continue
				}
				values := got.Values
				if len(values) == 0 && got.Value != "" {
					values = []string{got.Value}
				}
				if question, known := offeredQuestions[item.ID]; known {
					values = matchOptions(question, values)
				}
				put(domain.Answer{
					QuestionID: item.ID, Prompt: item.Prompt, Type: domain.AnswerType(item.Type),
					Values: values, Skipped: got.Skipped || len(values) == 0,
				})
			}
		}
	}
}

// offeredVocabulary is the bank's chips and questions for this turn's medium, keyed the way askDescribe and
// askQuestions key them: a category's chip label or value onto its value, and a question by its id.
func (s *RunState) offeredVocabulary(ctx context.Context) (map[string]map[string]string, map[string]banks.Question) {
	medium := s.reviewMedium(ctx)
	if medium == "" || s.Agent.banks == nil {
		return nil, nil
	}
	// The same request askReviewer makes, covered topics included: a covered topic changes which questions are
	// offered, and a question missing here would keep the label she tapped instead of its value.
	selection, err := banks.Select(s.Agent.banks, banks.Request{
		Medium: medium, Format: s.MediumFormat(), CoveredTopics: s.PreRead.Topics,
	})
	if err != nil {
		s.Agent.log.Warn("the question bank could not be read for her earlier answers", "err", err.Error())
		return nil, nil
	}
	chips := make(map[string]map[string]string, len(selection.Categories))
	for _, category := range selection.Categories {
		values := make(map[string]string, len(category.Chips))
		for _, chip := range category.Chips {
			values[chip.Value] = chip.Value
			values[chip.Label] = chip.Value
		}
		chips[category.Name] = values
	}
	questions := make(map[string]banks.Question, len(selection.Questions))
	for _, question := range selection.Questions {
		questions[question.ID] = question
	}
	return chips, questions
}

// mapThrough keeps the values the offered set knows, in the order they were given. Anything else is not a chip she
// could have tapped.
func mapThrough(values []string, offered map[string]string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if mapped, ok := offered[value]; ok {
			kept = append(kept, mapped)
		}
	}
	return kept
}

// reviewMedium is the medium the questions are chosen for: this turn's, or the card's work when the pre-read did not
// name one (it is off, or decisions is not configured). askReviewer and rememberAnswers both read it here, so the two
// cannot disagree about which questions, and which options, exist.
func (s *RunState) reviewMedium(ctx context.Context) string {
	if s.Medium != "" {
		return s.Medium
	}
	if s.Draft != nil {
		if work, err := s.Agent.store.Work(ctx, s.Draft.Entry.WorkID); err == nil {
			return work.Medium
		}
	}
	return ""
}

// answered reports whether this turn already holds an answer for a question, so it is not offered again.
func (s *RunState) answered(questionID string) bool {
	for _, answer := range s.Answers {
		if answer.QuestionID == questionID {
			return true
		}
	}
	return false
}

// waitForHer blocks on her answer with the turn's deadline stopped. Only an explicit stop, which cancels the
// context, ends the wait early.
func (s *RunState) waitForHer(ctx context.Context, interactionID string) (json.RawMessage, error) {
	s.clock.pause()
	defer s.clock.resume()
	return s.Agent.responder.Wait(ctx, s.SessionID, interactionID)
}

// askAny opens a question that is not part of the bank, such as a fill_gap, still under the round cap.
func (s *RunState) askAny(ctx context.Context, kind string, payload any) (json.RawMessage, error) {
	if s.Agent.responder == nil {
		return nil, ErrNoResponder
	}
	question, err := s.Agent.session.Ask(ctx, s.SessionID, kind, payload)
	if err != nil {
		return nil, err
	}
	return s.waitForHer(ctx, question.ID)
}

// onMessage keeps what nova produced, without writing it anywhere yet. The whole transcript is stored once the
// turn ends, so a killed process loses no more than the turn it was in.
func (s *RunState) onMessage(m message.Message, _ *provider.Usage) error {
	if m.Synthetic || m.Role == message.RoleUser {
		return nil
	}
	s.transcript = append(s.transcript, m)
	return nil
}

// stallNudge returns the one nudge a turn that ended in prose should get, or "" when the turn did not stall. A
// turn that stopped for a budget or an error never stalls. Two shapes do:
//   - she picked a work and no card exists, so merge_candidates never ran;
//   - this turn started the card but the model never tried save_entry. A refused save counts as a try: a prose reply to a
//     refusal is the model talking to her, not a stall.
func (s *RunState) stallNudge() string {
	if s.ended() != "done" || s.isStopped() {
		return ""
	}
	if s.Chosen != nil && s.Draft == nil {
		return nudgePickInstruction
	}
	if s.Draft != nil && s.StartedCard && !s.SaveAttempted {
		return nudgeSaveInstruction
	}
	return ""
}

// guardCall bounds the number of tool steps and stops tools once the card has been built.
func (s *RunState) guardCall(_ context.Context, c novaagent.ToolCall) *novaagent.CallDecision {
	if len(c.Arguments) > maxToolArgs {
		return &novaagent.CallDecision{Block: true, Reason: "that tool call is too large"}
	}
	s.Steps++
	if s.Steps > s.Agent.maxSteps {
		s.markStopped()
		return &novaagent.CallDecision{Block: true, Reason: "the step limit for this turn has been reached"}
	}
	if s.SaveCalled && c.Name != "search_library" {
		return &novaagent.CallDecision{Block: true, Reason: "the card is already built; wait for her confirm"}
	}
	return nil
}

// checkInputBudget refuses a request whose transcript is already over the input cap. The estimate is deliberately
// rough (four characters a token) and errs high, which is the safe direction for a cap.
func (s *RunState) checkInputBudget(_ context.Context, msgs []message.Message) ([]message.Message, error) {
	chars := 0
	for _, m := range msgs {
		chars += len(m.Content) + len(m.Reasoning)
		for _, call := range m.ToolCalls {
			chars += len(call.Function.Arguments)
		}
	}
	if chars/4 > s.Agent.maxInputTokens {
		return msgs, errBudget
	}
	return msgs, nil
}

// endTurn stops the loop once the card is built and the model has had its one closing sentence. It also stops a
// turn that hit the budget, with the reason already written.
func (s *RunState) endTurn(_ context.Context, t novaagent.Turn) novaagent.TurnDecision {
	if s.ended() == "budget" || s.isStopped() {
		return novaagent.TurnDecision{End: true}
	}
	if s.SaveCalled && len(t.Message.ToolCalls) == 0 {
		return novaagent.TurnDecision{End: true}
	}
	return novaagent.TurnDecision{}
}

// saveTranscript stores the conversation itself, which outlives the event log. User messages are the same ones
// the session already holds; the assistant and tool messages are this turn's.
func (s *RunState) saveTranscript(ctx context.Context, users []string) error {
	messages := make([]message.Message, 0, len(users)+len(s.transcript))
	for _, text := range users {
		messages = append(messages, message.User(text))
	}
	messages = append(messages, s.transcript...)
	body, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("agent: encode transcript: %w", err)
	}
	return s.Agent.store.SetMessages(ctx, s.SessionID, body)
}
