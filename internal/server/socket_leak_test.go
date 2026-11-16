package server

import (
	"bufio"
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// A failed write used to end the writer without closing the hijacked connection, which net/http never closes. The
// connection leaked, and the read loop stayed blocked on it until its deadline.
func TestFailedWriteClosesTheConnection(t *testing.T) {
	pipe, theirs := net.Pipe()
	defer func() { _ = theirs.Close() }()
	// The peer stays open, so the only thing that can wake a blocked Read is the server closing its own side.
	ours := &failingWrites{Conn: pipe}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sock := &socket{
		ws:        &websocket{conn: ours, br: bufio.NewReader(ours), bw: bufio.NewWriter(ours)},
		persisted: make(chan []byte, 1), live: make(chan []byte, 1),
		closed: make(chan struct{}), ctx: ctx, cancel: cancel,
	}
	sock.persisted <- []byte(`{}`)
	sock.wg.Add(1)
	go sock.writeLoop()

	readDone := make(chan error, 1)
	go func() {
		_, err := ours.Read(make([]byte, 1)) // what the read loop is doing
		readDone <- err
	}()
	select {
	case err := <-readDone:
		if err == nil {
			t.Error("the read returned without an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the connection was still open after the write failed")
	}
	sock.wg.Wait()
}

// failingWrites is a connection whose writes always fail and whose reads block until it is closed.
type failingWrites struct{ net.Conn }

func (failingWrites) Write([]byte) (int, error) { return 0, errors.New("write failed") }
