package agent

import "testing"

func TestPluralAndProviderWords(t *testing.T) {
	cases := map[string]string{
		plural(0, "quote", "quotes"):   "0 quotes",
		plural(1, "quote", "quotes"):   "1 quote",
		plural(2, "quote", "quotes"):   "2 quotes",
		plural(1, "match", "matches"):  "1 match",
		providerLabel("openlibrary"):   "Open Library",
		providerLabel("tmdb"):          "TMDB",
		providerLabel("something-new"): "something-new",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
