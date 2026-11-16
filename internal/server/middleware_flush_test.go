package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A handler that flushes between writes must still produce one valid gzip stream. Closing the compressor on
// Flush made every later write fail and truncated the reply.
func TestCompressSurvivesAFlushMidReply(t *testing.T) {
	s := &Server{}
	handler := s.compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "first ")
		w.(http.Flusher).Flush()
		if _, err := io.WriteString(w, "second"); err != nil {
			t.Errorf("a write after Flush failed: %v", err)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	zr, err := gzip.NewReader(strings.NewReader(rec.Body.String()))
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("reading the stream: %v", err)
	}
	if string(got) != "first second" {
		t.Errorf("body = %q, want %q", got, "first second")
	}
}
