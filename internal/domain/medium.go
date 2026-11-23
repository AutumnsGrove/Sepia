package domain

import "strings"

// mediums is every kind of work Sepia holds. A medium is data, not a column: the facts that differ between a film and
// a book live in the work's details JSON, validated per medium (PLAN.md 6).
var mediums = []string{"book", "film", "tv", "game", "album", "podcast", "theatre", "other"}

// Mediums returns the known mediums, in the order the Library's chips show them.
func Mediums() []string { return append([]string(nil), mediums...) }

// ParseMedium reads a medium a client or a tool sent. The value is returned lowercased, and ok is false for anything
// that is not one of the eight, so a caller refuses rather than searching a medium that cannot exist.
func ParseMedium(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	for _, known := range mediums {
		if value == known {
			return value, true
		}
	}
	return "", false
}
