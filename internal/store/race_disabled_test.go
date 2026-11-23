//go:build !race

package store

// raceDetector is false in an ordinary build, where the wall-clock budgets are asserted.
const raceDetector = false

// searchBenchEntries is the size of the invented library the budget test seeds: ten times the largest personal journal
// this app has seen, and enough that a query which scanned every body would be obvious.
const searchBenchEntries = 5000
