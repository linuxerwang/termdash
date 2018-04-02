// Package eventqueue provides an unboud FIFO queue of events.
package eventqueue

import (
	"context"
	"sync"
	"time"

	"github.com/mum4k/termdash/terminalapi"
)

// node is a single data item on the queue.
type node struct {
	next  *node
	event terminalapi.Event
}

// Unbound is an unbound FIFO queue of terminal events.
// Unbound must not be copied, pass it by reference only.
// This implementation is thread-safe.
type Unbound struct {
	first *node
	last  *node
	// mu protects first and last.
	mu sync.Mutex

	// cond is used to notify any callers waiting on a call to Pull().
	cond *sync.Cond

	// condMU protects cond.
	condMU sync.RWMutex

	// done is closed when the queue isn't needed anymore.
	done chan struct{}
}

// New returns a new Unbound queue of terminal events.
// Call Close() when done with the queue.
func New() *Unbound {
	u := &Unbound{
		done: make(chan (struct{})),
	}
	u.cond = sync.NewCond(&u.condMU)
	go u.wake() // Stops when Close() is called.
	return u
}

// wake periodically wakes up a goroutine waiting at Pull() so it can
// check if the context expired.
func (u *Unbound) wake() {
	const spinTime = 250 * time.Millisecond
	t := time.NewTicker(spinTime)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			u.cond.Signal()
		case <-u.done:
			return
		}
	}
}

// empty determines if the queue is empty.
func (u *Unbound) empty() bool {
	return u.first == nil
}

// Put puts an event onto the queue.
func (u *Unbound) Push(e terminalapi.Event) {
	u.mu.Lock()
	defer u.mu.Unlock()

	n := &node{
		event: e,
	}
	if u.empty() {
		u.first = n
		u.last = n
	} else {
		u.last.next = n
		u.last = n
	}
	u.cond.Signal()
}

// Get gets an event from the queue. Returns nil if the queue is empty.
func (u *Unbound) Pop() terminalapi.Event {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.empty() {
		return nil
	}

	n := u.first
	u.first = u.first.next

	if u.empty() {
		u.last = nil
	}
	return n.event
}

// Pull is like Pop(), but blocks until an item is available or the context
// expires.
func (u *Unbound) Pull(ctx context.Context) (terminalapi.Event, error) {
	if e := u.Pop(); e != nil {
		return e, nil
	}

	u.cond.L.Lock()
	defer u.cond.L.Unlock()
	for {
		if e := u.Pop(); e != nil {
			return e, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		u.cond.Wait()
	}
}

// Close should be called when the queue isn't needed anymore.
func (u *Unbound) Close() {
	close(u.done)
}