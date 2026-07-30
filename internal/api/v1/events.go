package v1

import (
	"github.com/satoshi-redline/redline/internal/engine"
	"sync"
)

type eventHub struct {
	mu   sync.RWMutex
	next uint64
	subs map[uint64]chan engine.Event
}

func newEventHub() *eventHub { return &eventHub{subs: map[uint64]chan engine.Event{}} }
func (h *eventHub) Emit(e engine.Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
func (h *eventHub) subscribe() (uint64, <-chan engine.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	ch := make(chan engine.Event, 32)
	h.subs[h.next] = ch
	return h.next, ch
}
func (h *eventHub) unsubscribe(id uint64) { h.mu.Lock(); delete(h.subs, id); h.mu.Unlock() }
