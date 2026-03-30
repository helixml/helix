package tui

import (
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// OutboxEntry is a queued message waiting to be sent.
type OutboxEntry struct {
	ID        string                    // Client-side unique ID
	Request   *types.SessionChatRequest // The chat request to send
	CreatedAt time.Time
	Attempts  int
	LastError error
	Status    OutboxStatus
}

// OutboxStatus tracks where a message is in the send pipeline.
type OutboxStatus int

const (
	OutboxPending  OutboxStatus = iota // Waiting to send
	OutboxSending                      // Currently being sent
	OutboxSent                         // Successfully sent
	OutboxFailed                       // Failed after retries
)

// Outbox queues messages for reliable delivery.
// Messages are sent in order. If the API is unreachable, messages
// queue locally and are flushed when the connection resumes.
type Outbox struct {
	mu      sync.Mutex
	entries []*OutboxEntry
	nextID  int
}

func NewOutbox() *Outbox {
	return &Outbox{}
}

// Enqueue adds a message to the outbox. Returns the client-side ID.
func (o *Outbox) Enqueue(req *types.SessionChatRequest) string {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.nextID++
	id := time.Now().Format("20060102150405") + "-" + string(rune('a'+o.nextID%26))

	entry := &OutboxEntry{
		ID:        id,
		Request:   req,
		CreatedAt: time.Now(),
		Status:    OutboxPending,
	}
	o.entries = append(o.entries, entry)
	return id
}

// NextPending returns the next pending message to send, or nil.
func (o *Outbox) NextPending() *OutboxEntry {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, e := range o.entries {
		if e.Status == OutboxPending {
			return e
		}
	}
	return nil
}

// MarkSending marks a message as currently being sent.
func (o *Outbox) MarkSending(id string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, e := range o.entries {
		if e.ID == id {
			e.Status = OutboxSending
			e.Attempts++
			return
		}
	}
}

// MarkSent marks a message as successfully sent.
func (o *Outbox) MarkSent(id string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, e := range o.entries {
		if e.ID == id {
			e.Status = OutboxSent
			return
		}
	}
}

// MarkFailed marks a message as failed. If under max retries, requeues it.
func (o *Outbox) MarkFailed(id string, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, e := range o.entries {
		if e.ID == id {
			e.LastError = err
			if e.Attempts >= 3 {
				e.Status = OutboxFailed
			} else {
				e.Status = OutboxPending // retry
			}
			return
		}
	}
}

// PendingCount returns the number of unsent messages.
func (o *Outbox) PendingCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()

	count := 0
	for _, e := range o.entries {
		if e.Status == OutboxPending || e.Status == OutboxSending {
			count++
		}
	}
	return count
}

// Cleanup removes sent entries older than the given duration.
func (o *Outbox) Cleanup(maxAge time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	var kept []*OutboxEntry
	for _, e := range o.entries {
		if e.Status == OutboxSent && e.CreatedAt.Before(cutoff) {
			continue // drop old sent entries
		}
		kept = append(kept, e)
	}
	o.entries = kept
}
