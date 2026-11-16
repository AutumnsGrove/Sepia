package domain

import "testing"

// MirrorStatus is a closed set, and the database enforces the same three values with a CHECK constraint
// (internal/store/migrations/0001_init.sql). A status Valid accepts but the schema refuses would fail at the write,
// which is the worst place to find out, so the two lists are asserted together.
func TestMirrorStatusMatchesTheDatabaseCheck(t *testing.T) {
	for _, ok := range []MirrorStatus{MirrorOK, MirrorFailed, MirrorSkipped} {
		if !ok.Valid() {
			t.Errorf("%q is in the schema and not in Valid", ok)
		}
	}
	for _, bad := range []MirrorStatus{"", "OK", "done", " ok"} {
		if bad.Valid() {
			t.Errorf("%q is not in the schema and Valid accepted it", bad)
		}
	}
}
