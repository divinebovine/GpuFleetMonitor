package gpu

import "sync"

// Fan out broker
type Hub struct {
	mu   sync.RWMutex
	subs map[chan string]struct{}
}

var DefaultHub = &Hub{subs: make(map[chan string]struct{})}

func (h *Hub) Subscribe() chan string {
	ch := make(chan string, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *Hub) Publish(id string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- id:
		default:
		}
	}
}
