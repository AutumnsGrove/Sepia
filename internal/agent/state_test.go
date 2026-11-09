package agent

import (
	"context"
	"sync"
	"testing"

	novaagent "github.com/AutumnsGrove/nova/agent"
)

// Nova may call the TurnEnd hook and return from Run on different goroutines, so the turn's end state is guarded.
// This is the guard the race detector found missing under load (S11): endTurn read Ended and stopped while the
// turn wrote them. It is only meaningful under `go test -race`, which is how make test runs.
func TestTheTurnEndStateIsSafeWhileTheTurnFinishes(t *testing.T) {
	state := &RunState{}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			state.endTurn(context.Background(), novaagent.Turn{})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			state.setEnded("done")
			state.markStopped()
			_ = state.ended()
			_ = state.isStopped()
		}
	}()
	wg.Wait()
}
