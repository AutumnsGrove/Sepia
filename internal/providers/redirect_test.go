package providers

import (
	"net/http"
	"net/url"
	"testing"
)

func TestRedirectPolicy(t *testing.T) {
	policy := redirectPolicy(withDefaults(Config{}))
	hop := func(raw string) *http.Request {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Request{URL: u}
	}
	cases := []struct {
		name string
		to   string
		via  int
		ok   bool
	}{
		{"a configured provider host", "https://image.tmdb.org/t/p/w342/a.jpg", 1, true},
		{"the archive Open Library serves covers from", "https://ia800000.us.archive.org/1/a.jpg", 1, true},
		{"a host the config never named", "https://evil.example/a.jpg", 1, false},
		{"a look-alike of a named host", "https://image.tmdb.org.evil.example/a.jpg", 1, false},
		{"a downgrade to http", "http://image.tmdb.org/a.jpg", 1, false},
		{"loopback the config never named", "http://127.0.0.1:9000/a.jpg", 1, false},
		{"https loopback the config never named", "https://localhost/a.jpg", 1, false},
		{"too many hops", "https://image.tmdb.org/a.jpg", maxRedirects + 1, false},
	}
	// The dev world names a loopback base URL itself, and that one host stays reachable.
	dev := redirectPolicy(withDefaults(Config{TMDBCoverBase: "http://127.0.0.1:40297/covers"}))
	if err := dev(hop("http://127.0.0.1:40297/a.jpg"), make([]*http.Request, 1)); err != nil {
		t.Errorf("the loopback host the config named was refused: %v", err)
	}
	for _, c := range cases {
		via := make([]*http.Request, c.via)
		err := policy(hop(c.to), via)
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
