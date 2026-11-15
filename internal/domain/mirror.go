package domain

import "time"

// A Mirror is one receipt from the host backup script: what was copied, when, and whether it worked. It is
// append-only, like everything else that records what happened, and it never holds review text: a target is a path
// or a remote, a detail is a sentence about a failure (docs/BUILD.md invariant 5).
type Mirror struct {
	ID     string       `json:"id"`
	RanAt  time.Time    `json:"ran_at"`
	Target string       `json:"target"`
	Status MirrorStatus `json:"status"`
	Bytes  int64        `json:"bytes"`
	Detail string       `json:"detail"`
}

// MirrorStatus is the closed set the database enforces with a CHECK constraint. A new value needs a migration, not a
// string constant.
type MirrorStatus string

const (
	// MirrorOK means the target was verified after the copy: rclone read it back.
	MirrorOK MirrorStatus = "ok"
	// MirrorFailed means the run did not finish, and Detail says what went wrong.
	MirrorFailed MirrorStatus = "failed"
	// MirrorSkipped means there was nothing to copy, which is not a failure.
	MirrorSkipped MirrorStatus = "skipped"
)

// Valid reports whether the status is one the database will accept.
func (s MirrorStatus) Valid() bool {
	switch s {
	case MirrorOK, MirrorFailed, MirrorSkipped:
		return true
	}
	return false
}
