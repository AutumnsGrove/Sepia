// Package backup is the app's half of the backup flow: it reads the receipts the host script appends after a
// snapshot and a mirror, imports them into the mirrors table, and answers the Backups screen. It never takes a
// snapshot and never talks to R2 itself: the script, the timer and the rclone remote live on the host
// (docs/slices/S14-backups.md, Decisions), so a wedged container cannot stop a backup and the token never enters
// the container.
package backup

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// The bounds on what the app will read back. The file is written by our own script, but it sits on disk and a
// tampered or truncated one must cost bounded time and memory rather than a wedged screen
// (docs/STANDARDS.md 4a).
const (
	// MaxReceiptFileBytes is how much of the receipt file is read. The file is append-only, so the tail is the
	// newest runs, which is what the screen shows; two lines a night is about 400 bytes, so this is years.
	MaxReceiptFileBytes = 256 << 10
	// MaxReceipts caps how many receipts one read can yield, and therefore how many rows one import can write.
	MaxReceipts = 500
	// MaxReceiptLine is the longest line the script can write: a 1000-character detail plus escaping.
	MaxReceiptLine = 8 << 10
	// MaxDetail and MaxTarget are the contract's own limits (api/openapi.yaml), enforced again here so a hand-edited
	// file cannot put a longer string in the database or on the screen.
	MaxDetail = 1000
	MaxTarget = 200
	// MaxRunID bounds the run id the script stamps on every receipt.
	MaxRunID = 128
	// MaxReceiptBytes is the largest size a mirror can claim to have copied. The board has 60 GB; a number above
	// this is a corrupt line, not a backup.
	MaxReceiptBytes = 1 << 40
)

// Parsed is the result of reading the receipt file. A line the parser cannot trust is counted and skipped, never
// guessed at: one corrupt line must not hide the night that worked.
type Parsed struct {
	Items []domain.Mirror
	// Skipped is how many lines were not usable.
	Skipped int
	// Truncated is true when the file is larger than one bounded read. Only the newest receipts are seen.
	Truncated bool
}

// receiptLine is the shape the script writes. It is deliberately a separate type from domain.Mirror: this is the
// wire format of a file, and the domain type must not grow a JSON contract by accident.
type receiptLine struct {
	Run    string `json:"run"`
	RanAt  string `json:"ran_at"`
	Target string `json:"target"`
	Status string `json:"status"`
	Bytes  int64  `json:"bytes"`
	Detail string `json:"detail"`
}

// Parse reads the receipt file. It never fails: an unreadable line is skipped and counted, because a list of runs
// that stops at the first bad line would hide every night after it.
func Parse(r io.Reader) Parsed {
	var out Parsed
	// A read error part way through is the same as a short file: use what arrived, because the receipts that did
	// arrive are still true.
	body, _ := io.ReadAll(io.LimitReader(r, MaxReceiptFileBytes+1))
	if len(body) > MaxReceiptFileBytes {
		// More than one bounded read arrived: keep the part that was read and drop the half line at its end. The
		// caller is responsible for starting at the newest end of an append-only file (see tail).
		out.Truncated = true
		body = body[:MaxReceiptFileBytes]
		if cut := bytes.LastIndexByte(body, '\n'); cut >= 0 {
			body = body[:cut+1]
		}
	}

	seen := make(map[string]bool, MaxReceipts)
	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if len(line) > MaxReceiptLine {
			out.Skipped++
			continue
		}
		mirror, ok := parseLine(line)
		if !ok {
			out.Skipped++
			continue
		}
		if seen[mirror.ID] {
			// The import id is the run plus the target, so this is the same record twice. The database keeps the
			// first; the parser agrees with it rather than showing one run twice.
			continue
		}
		seen[mirror.ID] = true
		out.Items = append(out.Items, mirror)
	}

	if len(out.Items) > MaxReceipts {
		out.Items = out.Items[len(out.Items)-MaxReceipts:]
	}
	return out
}

// parseLine turns one JSON line into a receipt, or reports that it cannot be trusted.
func parseLine(line string) (domain.Mirror, bool) {
	var raw receiptLine
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return domain.Mirror{}, false
	}
	run := clean(raw.Run, MaxRunID)
	target := clean(raw.Target, MaxTarget)
	detail := clean(raw.Detail, MaxDetail)
	if run == "" || target == "" {
		return domain.Mirror{}, false
	}
	status := domain.MirrorStatus(raw.Status)
	if !status.Valid() {
		return domain.Mirror{}, false
	}
	if raw.Bytes < 0 || raw.Bytes > MaxReceiptBytes {
		return domain.Mirror{}, false
	}
	ranAt, err := time.Parse(time.RFC3339, raw.RanAt)
	if err != nil {
		return domain.Mirror{}, false
	}
	return domain.Mirror{
		ID: run + ":" + target, RanAt: ranAt.UTC(), Target: target, Status: status,
		Bytes: raw.Bytes, Detail: detail,
	}, true
}

// clean strips what must not reach a log or a screen, then bounds the length. A newline or a control character in a
// receipt would let a hand-edited file reshape the panel; invalid UTF-8 is replaced rather than refused, because a
// mangled byte is not worth hiding a night's backup over.
func clean(value string, limit int) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r == utf8.RuneError:
			b.WriteRune('?')
		case r < 0x20 || r == 0x7f:
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	value = strings.TrimSpace(b.String())
	if utf8.RuneCountInString(value) > limit {
		runes := []rune(value)
		value = string(runes[:limit])
	}
	return value
}
