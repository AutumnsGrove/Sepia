package world

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AutumnsGrove/nova/provider"
)

// Chat is the fake OpenRouter chat endpoint. It speaks the same server-sent-event stream the real API does, so an
// agent test drives nova's real openaicompat provider and the real agent loop; only the model is scripted.
//
// A scripted turn is one reply. A turn with tool calls makes the loop execute them and come back for the next
// turn, which is how a whole review flow is replayed in order. When the script runs out the fake answers with a
// short final line, so a test that only cares about the first turn stays readable.
type Chat struct {
	*Service

	mu        sync.Mutex
	turns     []ChatTurn
	chatTurns []ChatTurn
	next      int
	bodies    [][]byte
	keepRaw   bool
}

// ChatToolCall is one tool call in a scripted turn. Arguments is the raw JSON string the model would send, so a
// test can script malformed arguments on purpose.
type ChatToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ChatTurn is one scripted reply.
type ChatTurn struct {
	Text      string
	ToolCalls []ChatToolCall
	// Finish is "stop" or "tool_calls"; an empty value is inferred from whether the turn has tool calls.
	Finish string
	// Usage is reported with the turn. Zero means a small, cheap default, so a test that does not care about
	// tokens still sees a cost row.
	Usage provider.Usage
}

// NewChat builds the fake with a fixed clock when the caller gives none.
func NewChat(name string, clock Clock) *Chat {
	return &Chat{Service: NewService(name, clock)}
}

// Script replaces the scripted turns.
func (c *Chat) Script(turns ...ChatTurn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turns = append([]ChatTurn(nil), turns...)
	c.next = 0
}

// ScriptChat replaces the scripted turns for a library chat, which is a different conversation from a card: its tools
// are the chat's, and a card script replayed into it would call tools it does not have. An empty chat script leaves the
// fake's own chat behaviour, which searches for the question and says what it found.
func (c *Chat) ScriptChat(turns ...ChatTurn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.chatTurns = append([]ChatTurn(nil), turns...)
}

// Turns returns how many scripted turns have been served.
func (c *Chat) Turns() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.next
}

// KeepBodies asks the fake to keep the request bodies. It is off by default, because a recorder must never become a
// copy of what she wrote; a test that asserts what the model saw turns it on with synthetic fixtures only.
func (c *Chat) KeepBodies(keep bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keepRaw = keep
}

// Bodies returns the kept request bodies, oldest first.
func (c *Chat) Bodies() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.bodies))
	for i, body := range c.bodies {
		out[i] = append([]byte(nil), body...)
	}
	return out
}

// ServeHTTP answers one chat-completions request.
func (c *Chat) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fault := c.Fault(r.URL.Path)
	c.record(r)
	body := readBounded(r, 64<<10)
	if c.keepRaw {
		c.mu.Lock()
		c.bodies = append(c.bodies, body)
		c.mu.Unlock()
	}

	switch fault {
	case FaultSlow:
		select {
		case <-time.After(slowDelay):
		case <-r.Context().Done():
			return
		}
	case FaultError:
		writeJSONError(w, statusError, c.Name()+" is unavailable")
		return
	case FaultRateLimit:
		if c.RetryAfter() > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(c.RetryAfter().Seconds())))
		}
		writeJSONError(w, http.StatusTooManyRequests, c.Name()+" asks you to slow down")
		return
	case FaultNotFound:
		writeJSONError(w, http.StatusNotFound, "no such model")
		return
	case FaultDrop:
		dropConnection(w)
		return
	case FaultMalformed:
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: {\"choices\": not json\n\n")
		return
	case FaultStall:
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: {\"choices\":[")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		return
	case FaultHuge:
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: ")
		_, _ = w.Write(make([]byte, hugeBody))
		_, _ = fmt.Fprint(w, "\n\n")
		return
	case FaultNone:
	}

	// A library chat asks for a different set of tools, so the fake answers it as a chat: it searches for the words it
	// was asked about, then says what it found. That is a believable answer for the seeded dev library, and it keeps
	// one script from having to be two.
	var turn ChatTurn
	if chatRequest(body) {
		// A chat has its own script, or the fake's own behaviour. A card script is never replayed into it: the tools a
		// chat is given are not the tools a card is given.
		turn = c.chatTurnAt(turnIndex(body), lastUserText(body))
	} else {
		turn = c.takeAt(turnIndex(body))
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(w, format, args...)
		if flusher != nil {
			flusher.Flush()
		}
	}

	model := c.Name()
	chunk := func(delta map[string]any, finish string, usage *provider.Usage) {
		payload := map[string]any{
			"id": "chatcmpl-fake", "object": "chat.completion.chunk", "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		}
		if usage != nil {
			// The wire spells the reported cost "cost", not provider.Usage's JSON name, so the fake writes the
			// wire shape rather than reusing the struct's tags.
			wireUsage := map[string]any{
				"prompt_tokens": usage.PromptTokens, "completion_tokens": usage.CompletionTokens,
				"total_tokens": usage.TotalTokens, "cost": usage.CostUSD,
			}
			if usage.CachedTokens != 0 || usage.CacheWriteTokens != 0 {
				wireUsage["prompt_tokens_details"] = map[string]any{
					"cached_tokens": usage.CachedTokens, "cache_write_tokens": usage.CacheWriteTokens,
				}
			}
			if usage.ReasoningTokens != 0 {
				wireUsage["completion_tokens_details"] = map[string]any{"reasoning_tokens": usage.ReasoningTokens}
			}
			payload["choices"] = []any{}
			payload["usage"] = wireUsage
		}
		body, _ := json.Marshal(payload)
		write("data: %s\n\n", body)
	}

	if turn.Text != "" {
		chunk(map[string]any{"role": "assistant", "content": turn.Text}, "", nil)
	}
	for i, call := range turn.ToolCalls {
		chunk(map[string]any{"tool_calls": []any{map[string]any{
			"index": i, "id": call.ID, "type": "function",
			"function": map[string]any{"name": call.Name, "arguments": call.Arguments},
		}}}, "", nil)
	}
	finish := turn.Finish
	if finish == "" {
		if len(turn.ToolCalls) > 0 {
			finish = "tool_calls"
		} else {
			finish = "stop"
		}
	}
	chunk(map[string]any{}, finish, nil)

	usage := turn.Usage
	if usage == (provider.Usage{}) {
		usage = provider.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110, CostUSD: 0.0001, CostSource: "provider"}
	}
	chunk(map[string]any{}, "", &usage)
	write("data: [DONE]\n\n")
}

// chatTool is the tool only a library chat is given, so its presence is what makes a request a chat.
const chatTool = "edit_entry"

// chatRequest reports whether the request was sent to a library chat rather than to a card.
func chatRequest(body []byte) bool {
	var request struct {
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return false
	}
	for _, tool := range request.Tools {
		if tool.Function.Name == chatTool {
			return true
		}
	}
	return false
}

// lastUserText is the question the chat was asked, which is what it searches for. A request that cannot be read
// searches for nothing, and the fake's search then finds nothing rather than guessing.
func lastUserText(body []byte) string {
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return ""
	}
	for i := len(request.Messages) - 1; i >= 0; i-- {
		if request.Messages[i].Role == "user" && request.Messages[i].Content != "" {
			text := strings.TrimSpace(request.Messages[i].Content)
			if len(text) > 200 {
				text = text[:200]
			}
			return text
		}
	}
	return ""
}

// chatTurnAt is a chat's scripted turn, or the fake's own behaviour: search for what she asked about, then say what
// came back. There is no second script, because a chat is one question and one answer as far as the app is concerned.
func (c *Chat) chatTurnAt(index int, question string) ChatTurn {
	c.mu.Lock()
	script := append([]ChatTurn(nil), c.chatTurns...)
	c.mu.Unlock()
	if len(script) > 0 {
		if index < 0 || index >= len(script) {
			return ChatTurn{Text: "Done."}
		}
		return script[index]
	}
	if index == 0 {
		query, _ := json.Marshal(map[string]string{"query": question})
		return ChatTurn{ToolCalls: []ChatToolCall{{ID: "chat-search", Name: "search_library", Arguments: string(query)}}}
	}
	return ChatTurn{Text: "Here is what I found."}
}

// takeAt returns the scripted turn for a conversation that has already had index assistant messages, or a clean
// final line when the script is exhausted.
//
// The index is read from the request rather than kept on the fake, so the script is per conversation and not per
// process. That is what lets one dev stack serve several sessions: the end-to-end suite opens a new one for each
// test, and each has to start at the first turn rather than wherever the last one stopped.
func (c *Chat) takeAt(index int) ChatTurn {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.turns) {
		return ChatTurn{Text: "Done."}
	}
	c.next = index + 1
	return c.turns[index]
}

// turnIndex counts the assistant messages a request already carries, which is the number of replies the
// conversation has had. A request that cannot be read is -1, and gets the same clean final line as an exhausted
// script.
func turnIndex(body []byte) int {
	var request struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return -1
	}
	count := 0
	for _, message := range request.Messages {
		if message.Role == "assistant" {
			count++
		}
	}
	return count
}

// readBounded reads at most limit bytes, so a hostile test body cannot make the fake allocate without bound.
func readBounded(r *http.Request, limit int64) []byte {
	if r.Body == nil {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, limit))
	return body
}

// writeJSONError writes the OpenRouter-shaped error object.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"message":%q,"code":%d}}`, message, status)
}

// ChatServer is the chat fake on a real loopback listener.
type ChatServer struct {
	*Chat
	http *httptest.Server
}

// StartChat mounts the fake on a loopback listener.
func StartChat(name string, clock Clock) *ChatServer {
	chat := NewChat(name, clock)
	ts := httptest.NewServer(chat)
	return &ChatServer{Chat: chat, http: ts}
}

// URL is the base URL, with the /v1 path the OpenRouter provider appends chat/completions to.
func (s *ChatServer) URL() string { return strings.TrimRight(s.http.URL, "/") + "/v1" }

// Close stops the listener.
func (s *ChatServer) Close() { s.http.Close() }
