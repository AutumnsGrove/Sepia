package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/world"
)

// S09.6, lifecycle: the routes the slice plan still owed after the flow tests were green. A client too slow to keep
// up is closed rather than shed; a reconnect storm loses and repeats nothing; a restart lets a turn finish or leaves
// her words in the log; a database failure answers a stable code; and the soak keeps the hub's maps bounded
// (docs/slices/S09-server.md, "Tests and exit").

// --- the outbound queues ---------------------------------------------------------------------------------

// A persisted event is the log, so it is never shed: a client that cannot drain it is closed with a stable code and
// resumes from its own last_seq (docs/STANDARDS.md 4a).
func TestSlowClientIsClosedRatherThanShed(t *testing.T) {
	sock := &socket{
		persisted: make(chan []byte, 4),
		live:      make(chan []byte, 4),
		closed:    make(chan struct{}),
		flushWait: 20 * time.Millisecond,
	}
	for i := 0; i < cap(sock.persisted); i++ {
		sock.persisted <- []byte("event")
	}

	if sock.sendPersisted([]byte("late")) {
		t.Fatal("a persisted event was queued into a full channel")
	}
	select {
	case <-sock.closed:
	default:
		t.Fatal("the socket was not closed when its persisted queue stayed full")
	}
	if sock.closeCode != closeRateLimited {
		t.Errorf("close code = %d, want %d so the client knows to replay", sock.closeCode, closeRateLimited)
	}
	if got := len(sock.persisted); got != cap(sock.persisted) {
		t.Errorf("the persisted queue holds %d events, want all %d: a persisted event was shed", got, cap(sock.persisted))
	}

	// A client with room is served, and nothing is closed.
	roomy := &socket{
		persisted: make(chan []byte, 4),
		live:      make(chan []byte, 4),
		closed:    make(chan struct{}),
		flushWait: 20 * time.Millisecond,
	}
	if !roomy.sendPersisted([]byte("event")) {
		t.Fatal("an event was refused when the queue had room")
	}
	select {
	case <-roomy.closed:
		t.Fatal("a socket with room was closed")
	default:
	}
}

// A live-only event is a moving display, so the oldest is shed and the newest is kept (docs/STANDARDS.md 4a).
func TestLiveEventsAreShedFromTheOldest(t *testing.T) {
	sock := &socket{
		persisted: make(chan []byte, 4),
		live:      make(chan []byte, 2),
		closed:    make(chan struct{}),
	}
	sock.live <- []byte("oldest")
	sock.live <- []byte("newer")

	sock.sendLive([]byte("newest"))

	if got := len(sock.live); got != 2 {
		t.Fatalf("the live queue holds %d events, want 2", got)
	}
	first, second := <-sock.live, <-sock.live
	if string(first) != "newer" || string(second) != "newest" {
		t.Errorf("live queue = %q, %q; want the oldest shed and the newest kept", first, second)
	}

	// A socket that is already closing takes nothing more.
	sock.close(closeNormal, "done")
	sock.sendLive([]byte("ignored"))
	if got := len(sock.live); got != 0 {
		t.Errorf("a closing socket queued %d live events", got)
	}
}

// --- a reconnect storm -----------------------------------------------------------------------------------

// Every socket on a session gets the same hello and the same replay, and the log stays contiguous across a storm of
// reconnects. The hub forgets every one of them.
func TestReconnectStormKeepsTheLogContiguous(t *testing.T) {
	rig := newSocketRig(t)
	defer rig.close()
	ctx := context.Background()
	sess, err := rig.engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const messages = 6
	for i := 1; i <= messages; i++ {
		if _, appendErr := rig.engine.AppendUserMessage(ctx, sess.ID, "message "+strconv.Itoa(i), "m"+strconv.Itoa(i)); appendErr != nil {
			t.Fatalf("AppendUserMessage %d: %v", i, appendErr)
		}
	}
	head, err := rig.engine.HeadSequence(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if head < messages {
		t.Fatalf("head = %d, want at least %d", head, messages)
	}

	// Wave one: many sockets open at once, which is what a phone reconnecting in a lift looks like.
	wave := make([]*wsClient, 0, 20)
	for i := 0; i < cap(wave); i++ {
		client := rig.dial(sess.ID, i%3, "http://"+rig.host)
		hello := client.expect("hello")
		if got := hello.field(t, "head_seq"); got != float64(head) {
			t.Errorf("socket %d: head_seq = %v, want %d", i, got, head)
		}
		wave = append(wave, client)
	}
	// Closing them all at once is the storm's second half.
	var wg sync.WaitGroup
	for _, client := range wave {
		wg.Add(1)
		go func(c *wsClient) {
			defer wg.Done()
			_ = c.conn.Close()
		}(client)
	}
	wg.Wait()

	// A resume from zero replays the whole conversation exactly once, in order.
	resumed := rig.dial(sess.ID, 0, "http://"+rig.host)
	resumed.expect("hello")
	seen := map[int]bool{}
	last := 0
	for last < head {
		ev, ok := resumed.next()
		if !ok {
			if ev.closeCode != 0 {
				t.Fatalf("the socket closed at seq %d with code %d", last, ev.closeCode)
			}
			t.Fatalf("the replay stalled at seq %d of %d", last, head)
		}
		if ev.Seq == nil {
			continue
		}
		if seen[*ev.Seq] {
			t.Fatalf("seq %d was replayed twice", *ev.Seq)
		}
		if *ev.Seq != last+1 {
			t.Fatalf("seq %d followed %d: the replay is not contiguous", *ev.Seq, last)
		}
		seen[*ev.Seq] = true
		last = *ev.Seq
	}

	// The hub has forgotten every socket, including this one once it closes.
	_ = resumed.conn.Close()

	// The hub has forgotten every socket, including the closed ones.
	waitFor(t, func() bool {
		conns, subs, turns := hubSnapshot(rig.inner)
		return conns == 0 && subs == 0 && turns == 0
	}, "the hub kept a socket or a turn after the storm")
}

// hubSnapshot reads the hub's three maps under its lock, so a test can say "nothing was kept" without the package
// growing a test-only method.
func hubSnapshot(s *Server) (conns, subs, turns int) {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	for _, set := range s.hub.bySession {
		subs += len(set)
	}
	return len(s.hub.conns), subs, len(s.hub.turns)
}

// waitFor polls a condition until it holds or a deadline passes. It is a poll rather than a sleep, so a fast machine
// is not punished and a loaded one is not flaky.
func waitFor(t *testing.T, ok func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(message)
}

// --- a restart in the middle of a turn -------------------------------------------------------------------

// gate holds every model call until the test releases it, so a turn is provably still running when the drain starts.
// It returns the handler and the release; a test that never releases must still call it at cleanup, or the blocked
// goroutine outlives the test.
func gate(handler http.Handler) (http.Handler, func()) {
	open := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(open) }) }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-open:
		case <-r.Context().Done():
			return
		}
		handler.ServeHTTP(w, r)
	}), release
}

// A turn already running when the service is asked to stop is given the grace period to finish: the socket is told
// the service is restarting, and the turn's ledger row is written before the drain returns (docs/PROTOCOL.md 1).
func TestRestartLetsTheTurnFinish(t *testing.T) {
	var release func()
	// A prose reply ends the turn after one nudge, so nothing here waits for a tap a restarting service cannot
	// deliver. The turn is held at the model, which is the only place the drain must wait.
	rig := newFlowRigGated(t, func(chat http.Handler) http.Handler {
		gated, rel := gate(chat)
		release = rel
		return gated
	}, world.ChatTurn{Text: "Noted."})
	defer rig.close()
	defer release()
	ctx := context.Background()

	sess, err := rig.engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := rig.dial(sess.ID, 0, "http://"+rig.host)
	client.expect("hello")
	client.sendJSON(map[string]any{
		"type": "message", "client_msg_id": "m1",
		"data": map[string]any{
			"text": world.SampleMessage, "client_date": "2026-11-01", "tz": "Europe/London",
		},
	})
	if got := client.expect("ack").field(t, "status"); got != "accepted" {
		t.Fatalf("message ack = %v", got)
	}

	// Start the drain the way Run does. It closes the sockets first and only then waits for the turn, so the client
	// learns immediately that the service is restarting.
	drained := make(chan struct{})
	go func() {
		rig.inner.teardown(5 * time.Second)
		close(drained)
	}()

	closed := client.expectClose(3 * time.Second)
	if closed.closeCode != closeGoingAway {
		t.Errorf("close code = %d, want %d so the client tells a restart from a drop", closed.closeCode, closeGoingAway)
	}
	select {
	case <-drained:
		t.Fatal("the drain returned while the turn was still blocked on the model")
	case <-time.After(150 * time.Millisecond):
	}

	// Releasing the model lets the turn finish, and the drain waits for it rather than cutting it off.
	release()
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("the drain did not return after the turn finished")
	}

	events, err := rig.engine.EventsSince(ctx, sess.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sawMessage bool
	for _, ev := range events {
		if ev.Kind == "user_message" {
			sawMessage = true
		}
	}
	if !sawMessage {
		t.Fatalf("her message is not in the log after the drain: %d events", len(events))
	}
	// The ledger row is written after the model has run and before the turn goroutine returns, so its presence is
	// the turn proving it finished inside the grace period rather than being cut off.
	calls, err := rig.db.TurnCalls(ctx, sess.ID)
	if err != nil {
		t.Fatalf("TurnCalls: %v", err)
	}
	if len(calls) == 0 {
		t.Fatal("the turn wrote no ledger row: it did not finish before the drain returned")
	}
	if _, _, turns := hubSnapshot(rig.inner); turns != 0 {
		t.Errorf("the hub still holds %d turns after the drain", turns)
	}
}

// The other half of the promise: a turn still blocked when the grace runs out does not hold the service open for
// ever, and her words are already in the log because they were stored when the socket accepted them.
func TestRestartAbandonsATurnPastTheGrace(t *testing.T) {
	var release func()
	rig := newFlowRigGated(t, func(chat http.Handler) http.Handler {
		gated, rel := gate(chat)
		release = rel
		return gated
	}, world.ChatTurn{Text: "Noted."})
	defer rig.close()
	defer func() {
		release()
		// Let the abandoned turn finish before the store closes, so the test cleans up after itself.
		done := make(chan struct{})
		go func() { rig.inner.hub.turnsWG.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}()
	ctx := context.Background()

	sess, err := rig.engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := rig.dial(sess.ID, 0, "http://"+rig.host)
	client.expect("hello")
	client.sendJSON(map[string]any{
		"type": "message", "client_msg_id": "m1",
		"data": map[string]any{
			"text": world.SampleMessage, "client_date": "2026-11-01", "tz": "Europe/London",
		},
	})
	client.expect("ack")

	start := time.Now()
	rig.inner.teardown(50 * time.Millisecond)
	if spent := time.Since(start); spent > 3*time.Second {
		t.Fatalf("the drain took %s with a 50ms grace", spent)
	}

	events, err := rig.engine.EventsSince(ctx, sess.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sawMessage bool
	for _, ev := range events {
		if ev.Kind == "user_message" {
			sawMessage = true
		}
	}
	if !sawMessage {
		t.Fatalf("her message is not in the log after an abandoned turn: %d events", len(events))
	}
}

// --- a database failure ---------------------------------------------------------------------------------

// A database that has gone away answers the contract's stable internal code and an envelope, never a driver string
// and never a panic. The reads and the writes are both checked, because a save path that fails differently is how a
// review gets lost (docs/STANDARDS.md 4).
func TestDatabaseFailureIsAStableCode(t *testing.T) {
	rig := newSocketRig(t)
	defer rig.close()
	ctx := context.Background()
	sess := rig.newDraft()
	if err := rig.db.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}

	cases := []struct {
		name, method, target, body string
	}{
		{"session list", http.MethodGet, "/api/sessions", ""},
		{"one session", http.MethodGet, "/api/sessions/" + sess.ID, ""},
		{"entry list", http.MethodGet, "/api/entries", ""},
		{"delete", http.MethodPost, "/api/entries/" + sess.ID + "/delete", `{"base_version":0}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(ctx, tc.method, rig.server.URL+tc.target, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Host = rig.host
			req.Header.Set("Origin", "http://"+rig.host)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500; body %s", resp.StatusCode, body)
			}
			var env envelope
			if err := json.Unmarshal(body, &env); err != nil {
				t.Fatalf("body is not the envelope: %v (%s)", err, body)
			}
			if env.Code != "internal" {
				t.Errorf("code = %q, want internal", env.Code)
			}
			lower := strings.ToLower(string(body))
			if strings.Contains(lower, "database") || strings.Contains(lower, "sql") {
				t.Errorf("the body describes the driver error: %s", body)
			}
		})
	}

	// A socket on a dead database is an HTTP refusal before the upgrade, not a hijacked connection that dies.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rig.server.URL+"/ws/sessions/"+sess.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = rig.host
	req.Header.Set("Origin", "http://"+rig.host)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Key", "0123456789abcdef")
	req.Header.Set("Sec-WebSocket-Version", "13")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("socket status = %d, want 500", resp.StatusCode)
	}
}

// --- the bounded soak -----------------------------------------------------------------------------------

// Many sessions, many reconnects, many pings: the hub's maps end up empty and no turn is left behind. This is the
// soak from the slice plan, bounded so it runs on every push rather than on a special occasion.
func TestSoakManyReconnectsStaysBounded(t *testing.T) {
	rig := newSocketRig(t)
	defer rig.close()
	ctx := context.Background()

	sessions := make([]domain.Session, 0, 8)
	for i := 0; i < cap(sessions); i++ {
		sess, err := rig.engine.CreateEntry(ctx)
		if err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, sess)
	}

	for round := 0; round < 8; round++ {
		for _, sess := range sessions {
			client := rig.dial(sess.ID, 0, "http://"+rig.host)
			client.expect("hello")
			client.sendJSON(map[string]any{
				"type": "ping", "client_msg_id": "p" + strconv.Itoa(round),
				// Every client message carries data, even when it has nothing in it (api/events.schema.json).
				"data": map[string]any{},
			})
			if got := client.expect("ack").field(t, "status"); got != "accepted" {
				t.Fatalf("ping ack = %v", got)
			}
			_ = client.conn.Close()
		}
	}

	waitFor(t, func() bool {
		conns, subs, turns := hubSnapshot(rig.inner)
		return conns == 0 && subs == 0 && turns == 0
	}, "the soak left a socket or a turn in the hub")

	// The rate limiter is the fourth bound and must not grow without limit: a session that has been quiet for a
	// minute is forgotten when the limiter next sweeps.
	limiter := rig.inner.hub.limiter
	limiter.mu.Lock()
	kept := len(limiter.buckets)
	limiter.mu.Unlock()
	if kept > len(sessions) {
		t.Errorf("the rate limiter kept %d buckets for %d sessions", kept, len(sessions))
	}
}

// expectClose reads until the socket's close frame, failing if the wait runs out first.
func (c *wsClient) expectClose(wait time.Duration) event {
	c.t.Helper()
	for i := 0; i < 200; i++ {
		ev, ok := c.nextWithin(wait)
		if !ok {
			if ev.closeCode != 0 {
				return ev
			}
			c.t.Fatalf("the socket closed without a close frame")
		}
	}
	c.t.Fatalf("no close frame arrived")
	return event{}
}
