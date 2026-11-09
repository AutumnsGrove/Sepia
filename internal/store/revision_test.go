package store

import (
	"context"
	"testing"
)

// The revision target is the session's record of what its working copy revises. It is written when the copy is
// made, read by confirm after a restart, and absent on a session that is not a revision.
func TestRevisionTargetRoundTrips(t *testing.T) {
	s, _ := draftRig(t)
	ctx := context.Background()

	empty, err := s.RevisionTarget(ctx, "s1")
	if err != nil {
		t.Fatalf("RevisionTarget: %v", err)
	}
	if empty != "" {
		t.Fatalf("target on a plain draft = %q, want empty", empty)
	}

	if err := s.LinkRevisionTarget(ctx, "s1", "e1"); err != nil {
		t.Fatalf("LinkRevisionTarget: %v", err)
	}
	target, err := s.RevisionTarget(ctx, "s1")
	if err != nil {
		t.Fatalf("RevisionTarget: %v", err)
	}
	if target != "e1" {
		t.Fatalf("target = %q, want e1", target)
	}

	// A session that does not exist reads as no revision rather than as a failure.
	if missing, err := s.RevisionTarget(ctx, "s-missing"); err != nil || missing != "" {
		t.Fatalf("missing session = %q, %v", missing, err)
	}
}
