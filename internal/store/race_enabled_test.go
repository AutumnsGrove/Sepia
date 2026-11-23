//go:build race

package store

// raceDetector is true in a `go test -race` build. The wall-clock budgets in search_budget_test.go are skipped there,
// because the runtime slows this read by an order of magnitude and the number would measure the detector.
const raceDetector = true

// searchBenchEntries is the size of the invented library the budget test seeds. It is smaller under the race detector
// because the detector's cost is per memory access: five thousand entries take minutes to seed and measure nothing a
// smaller library does not.
const searchBenchEntries = 300
