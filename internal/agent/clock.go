package agent

import (
	"sync"
	"time"
)

// turnClock is the turn's deadline, with a pause. The two minutes are for the model and the services, not for her:
// a turn that opens a "which one?" card must not be timed out because she spent a while deciding. The clock stops
// while a question is waiting and picks up with the time it had left, so the bound on everything else is unchanged.
//
// It runs on real time on purpose. It guards a real wait, so a test's fixed clock must not freeze it.
type turnClock struct {
	mu        sync.Mutex
	timer     *time.Timer
	remaining time.Duration
	started   time.Time
	paused    int
	expired   bool
	fire      func()
}

// newTurnClock starts a deadline of d. fire runs once if it is ever reached.
func newTurnClock(d time.Duration, fire func()) *turnClock {
	c := &turnClock{remaining: d, fire: fire}
	c.mu.Lock()
	c.startLocked()
	c.mu.Unlock()
	return c
}

func (c *turnClock) startLocked() {
	c.started = time.Now()
	c.timer = time.AfterFunc(c.remaining, func() {
		c.mu.Lock()
		c.expired = true
		c.mu.Unlock()
		c.fire()
	})
}

// pause stops the clock until the matching resume. It is safe on a nil clock, which is how a tool run outside a
// turn (a test that builds a RunState by hand) behaves.
func (c *turnClock) pause() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused++
	if c.paused > 1 || c.timer == nil {
		return
	}
	if c.timer.Stop() {
		c.remaining -= time.Since(c.started)
		if c.remaining < time.Millisecond {
			c.remaining = time.Millisecond
		}
	}
	c.timer = nil
}

// resume restarts the clock with whatever time was left.
func (c *turnClock) resume() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused == 0 {
		return
	}
	c.paused--
	if c.paused > 0 || c.expired || c.timer != nil {
		return
	}
	c.startLocked()
}

// stop ends the clock for good.
func (c *turnClock) stop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

// reached reports whether the deadline ran out.
func (c *turnClock) reached() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.expired
}
