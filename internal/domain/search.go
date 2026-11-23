package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Search is how her own words are found again: the text is reduced here to words that can be asked of SQLite's
// FTS5 safely, and the ranking weights live with the query (docs/slices/S16-search-and-chat.md, Decisions 1).
//
// Nothing she types ever reaches FTS5's own syntax. FTS5 treats quotes, parentheses, *, ^, OR, NOT and NEAR as
// operators, so a review of "the (mostly) slow parts" would be a syntax error at best and a different question at
// worst. The parser here keeps only letters and digits, which cannot be an operator, and hands them back as quoted
// strings.

const (
	// MaxSearchText is how long a search may be. It is in the limits table and the API contract.
	MaxSearchText = 200
	// MaxSearchWords bounds how many words are asked of the index at once. A longer search is refused rather than
	// quietly cut, because a search that answers a shorter question than the one she asked is a lie (invariant 12).
	MaxSearchWords = 8
)

// The sentinels the REST layer maps to plain copy. They are errors rather than codes because the server already
// owns the wording of a refused request.
var (
	// ErrSearchEmpty is a search with nothing in it: only punctuation, or only spaces.
	ErrSearchEmpty = errors.New("domain: the search has no words in it")
	// ErrSearchTooLong is a search over MaxSearchText characters.
	ErrSearchTooLong = errors.New("domain: the search is too long")
	// ErrSearchTooManyWords is a search with more than MaxSearchWords words in it.
	ErrSearchTooManyWords = errors.New("domain: the search has too many words")
)

// SearchTerms is what the index is asked: the words, and the FTS5 expression built from them.
type SearchTerms struct {
	// Words are the words she typed, lowercased and de-duplicated, in the order she typed them.
	Words []string
	// Match is the FTS5 MATCH expression. Every word is quoted and they are joined with AND, so a second word
	// narrows the search rather than widening it.
	Match string
}

// The markers a snippet wraps a matched word in. They are Unicode private-use characters rather than HTML because a
// snippet is her own text: HTML would make the browser render whatever a review happens to contain, and a review of
// "<script>" must stay a sentence about a script. The UI splits on them and escapes the text between.
const (
	// SnippetOpen starts a highlighted word inside a snippet.
	SnippetOpen = "\uE000"
	// SnippetClose ends a highlighted word inside a snippet.
	SnippetClose = "\uE001"
)

// ParseSearchText reads what she typed. Only letters and digits survive, so no FTS5 operator can be smuggled through,
// and the surviving words are bounded in both count and length.
func ParseSearchText(raw string) (SearchTerms, error) {
	if utf8.RuneCountInString(raw) > MaxSearchText {
		return SearchTerms{}, ErrSearchTooLong
	}
	var (
		words []string
		word  strings.Builder
		seen  = map[string]bool{}
	)
	flush := func() {
		if word.Len() == 0 {
			return
		}
		value := strings.ToLower(word.String())
		word.Reset()
		if seen[value] {
			return
		}
		seen[value] = true
		words = append(words, value)
	}
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	if len(words) == 0 {
		return SearchTerms{}, ErrSearchEmpty
	}
	if len(words) > MaxSearchWords {
		return SearchTerms{}, ErrSearchTooManyWords
	}
	quoted := make([]string, 0, len(words))
	for _, value := range words {
		// The word holds letters and digits only, so it cannot contain the quote this would have to escape.
		quoted = append(quoted, `"`+value+`"`)
	}
	return SearchTerms{Words: words, Match: strings.Join(quoted, " AND ")}, nil
}
