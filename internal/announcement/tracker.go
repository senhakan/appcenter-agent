package announcement

import (
	"sync"
	"time"
)

type PendingAnnouncement struct {
	ID       int
	Title    string
	Message  string
	Priority string
	ShownAt  time.Time
}

type Tracker struct {
	mu      sync.Mutex
	pending map[int]*PendingAnnouncement
}

func NewTracker() *Tracker {
	return &Tracker{pending: make(map[int]*PendingAnnouncement)}
}

func (t *Tracker) Add(id int, title, message, priority string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending[id] = &PendingAnnouncement{
		ID:       id,
		Title:    title,
		Message:  message,
		Priority: priority,
		ShownAt:  time.Now().UTC(),
	}
}

func (t *Tracker) Remove(id int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, id)
}

func (t *Tracker) GetCriticalPending() []*PendingAnnouncement {
	t.mu.Lock()
	defer t.mu.Unlock()

	threshold := time.Now().UTC().Add(-5 * time.Minute)
	out := make([]*PendingAnnouncement, 0)
	for _, ann := range t.pending {
		if ann == nil {
			continue
		}
		if ann.Priority != "critical" {
			continue
		}
		if ann.ShownAt.After(threshold) {
			continue
		}
		copy := *ann
		out = append(out, &copy)
	}
	return out
}
