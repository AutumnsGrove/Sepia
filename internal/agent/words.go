package agent

import "fmt"

// plural puts a count in front of the right form of a word: "1 quote", "2 quotes". The activity lines are shown to
// her as written, so "1 quotes" reads as a mistake.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// providerLabel is the name a person knows a metadata service by. The id is what the code stores; it is not what
// the activity line should say.
func providerLabel(provider string) string {
	switch provider {
	case "tmdb":
		return "TMDB"
	case "openlibrary":
		return "Open Library"
	case "polaris":
		return "Polaris"
	case "manual":
		return "your own details"
	default:
		return provider
	}
}
