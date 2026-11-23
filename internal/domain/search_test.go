package domain

import (
	"errors"
	"strings"
	"testing"
	"unicode"
)

// The search parser is the boundary between her text and FTS5's syntax. These cases are the three routes: words she
// typed, words that look like operators, and the limits.

func TestParseSearchTextKeepsHerWords(t *testing.T) {
	terms, err := ParseSearchText("  The slow   burn, in Hades II ")
	if err != nil {
		t.Fatalf("ParseSearchText: %v", err)
	}
	if got := strings.Join(terms.Words, ","); got != "the,slow,burn,in,hades,ii" {
		t.Fatalf("words = %q", got)
	}
	if terms.Match != `"the" AND "slow" AND "burn" AND "in" AND "hades" AND "ii"` {
		t.Fatalf("match = %q", terms.Match)
	}
}

func TestParseSearchTextDropsRepeatsAndCase(t *testing.T) {
	terms, err := ParseSearchText("Dune dune DUNE")
	if err != nil {
		t.Fatalf("ParseSearchText: %v", err)
	}
	if terms.Match != `"dune"` {
		t.Fatalf("match = %q, want one word asked once", terms.Match)
	}
}

// Bad: FTS5 operators and SQL-shaped text are words, not instructions. None of them may reach the query raw.
func TestParseSearchTextTreatsOperatorsAsWords(t *testing.T) {
	cases := map[string]string{
		`" OR 1=1 --`:         `"or" AND "1"`,
		`NEAR(slow burn)`:     `"near" AND "slow" AND "burn"`,
		`slow* ^title -burn`:  `"slow" AND "title" AND "burn"`,
		`'; DROP TABLE x; --`: `"drop" AND "table" AND "x"`,
		`NOT slow`:            `"not" AND "slow"`,
	}
	for raw, want := range cases {
		terms, err := ParseSearchText(raw)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if terms.Match != want {
			t.Fatalf("%q: match = %q, want %q", raw, terms.Match, want)
		}
		for _, word := range terms.Words {
			for _, r := range word {
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
					t.Fatalf("%q: word %q holds %q, which FTS5 could read as syntax", raw, word, r)
				}
			}
		}
	}
}

// Bad: a search with no words in it is refused rather than run as an empty query, which FTS5 would treat as a syntax
// error and which answers a question she did not ask.
func TestParseSearchTextRefusesPunctuationOnly(t *testing.T) {
	for _, raw := range []string{"", "   ", `"*()^`, "---"} {
		if _, err := ParseSearchText(raw); !errors.Is(err, ErrSearchEmpty) {
			t.Fatalf("%q: err = %v, want ErrSearchEmpty", raw, err)
		}
	}
}

// Bad: the limits are refused with a sentinel rather than truncated.
func TestParseSearchTextRefusesOverTheLimits(t *testing.T) {
	if _, err := ParseSearchText(strings.Repeat("a", MaxSearchText+1)); !errors.Is(err, ErrSearchTooLong) {
		t.Fatalf("too long: err = %v", err)
	}
	// The limit is characters, not bytes: 200 accented letters are one under it.
	if _, err := ParseSearchText(strings.Repeat("é", MaxSearchText)); err != nil {
		t.Fatalf("200 accented characters: %v", err)
	}
	words := "one two three four five six seven eight nine"
	if _, err := ParseSearchText(words); !errors.Is(err, ErrSearchTooManyWords) {
		t.Fatalf("too many words: err = %v", err)
	}
	// Exactly at the word limit is still a search.
	atLimit := "one two three four five six seven eight"
	if _, err := ParseSearchText(atLimit); err != nil {
		t.Fatalf("%d words: %v", MaxSearchWords, err)
	}
}

func FuzzParseSearchText(f *testing.F) {
	for _, seed := range []string{
		"slow burn",
		`" OR 1=1 --`,
		`NEAR(a b)`,
		"hades 2",
		"",
		"\x00\x01",
		"é ü ß",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		terms, err := ParseSearchText(raw)
		if err != nil {
			switch {
			case errors.Is(err, ErrSearchEmpty), errors.Is(err, ErrSearchTooLong), errors.Is(err, ErrSearchTooManyWords):
				return
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if len(terms.Words) == 0 || len(terms.Words) > MaxSearchWords {
			t.Fatalf("%d words", len(terms.Words))
		}
		for _, word := range terms.Words {
			if word == "" {
				t.Fatal("an empty word")
			}
			for _, r := range word {
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
					t.Fatalf("word %q holds %q", word, r)
				}
			}
		}
		// Every quoted word is balanced, so the expression is never a syntax error.
		if strings.Count(terms.Match, `"`) != 2*len(terms.Words) {
			t.Fatalf("match %q has unbalanced quotes", terms.Match)
		}
	})
}
