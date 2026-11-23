package agent

import (
	"net/url"
	"testing"

	"github.com/AutumnsGrove/sepia/internal/providers"
)

// The candidate picker draws a cover beside each title. The page's CSP is img-src 'self' data:, so a payload that
// carries the provider's own URL draws nothing: every candidate showed its initials and she picked a work with no
// cover at all (issue #53). The card's thumb points at the app, which fetches the provider's copy.
func TestACandidatePointsItsCoverAtTheApp(t *testing.T) {
	remote := "https://covers.openlibrary.org/b/id/123-M.jpg"
	options := candidateOptions([]providers.Candidate{{
		Provider: providers.ProviderOpenLibrary, ProviderID: "OL1W",
		Title: "The Lantern Keeper", Medium: "book", ThumbURL: remote,
	}})
	if len(options) != 1 {
		t.Fatalf("options = %d, want one per candidate", len(options))
	}

	thumb, _ := options[0]["thumb_url"].(string)
	parsed, err := url.Parse(thumb)
	if err != nil {
		t.Fatalf("thumb_url = %q: %v", thumb, err)
	}
	if parsed.Host != "" || parsed.Scheme != "" {
		t.Errorf("thumb_url = %q, which names another origin than the app's", thumb)
	}
	if parsed.Path != "/api/thumb" {
		t.Errorf("thumb_url = %q, want the app's thumb route", thumb)
	}
	query := parsed.Query()
	if query.Get("provider") != providers.ProviderOpenLibrary || query.Get("id") != "OL1W" {
		t.Errorf("thumb_url = %q, want the provider and id it stands for", thumb)
	}
	if query.Get("src") != remote {
		t.Errorf("src = %q, want the provider's URL for the app to fetch", query.Get("src"))
	}

	// A candidate without a cover says so with nothing rather than a request that can only fail, so the card draws
	// its stand-in.
	bare := candidateOptions([]providers.Candidate{{
		Provider: providers.ProviderTMDB, ProviderID: "movie:101", Title: "The Lantern Keeper", Medium: "film",
	}})
	if thumb, _ := bare[0]["thumb_url"].(string); thumb != "" {
		t.Errorf("a candidate with no cover carries thumb_url = %q, want none", thumb)
	}
}
