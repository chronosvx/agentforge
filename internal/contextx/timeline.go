// This file contains the concurrency-safe snapshot timeline used by the UI.
package contextx

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ErrNotFound is returned when a snapshot ID does not exist in the timeline.
var ErrNotFound = errors.New("timeline: snapshot not found")

// Snapshot is a single point-in-time snapshot stored in a Timeline.
type Snapshot struct {
	ID        string
	Label     string
	Data      []byte
	CreatedAt time.Time
}

// Timeline stores an ordered list of snapshots and a current position.
type Timeline struct {
	mu      sync.Mutex
	nodes   []Snapshot
	current int
}

var idCounter atomic.Uint64

// New returns an empty Timeline.
func New() *Timeline {
	return NewTimeline()
}

// NewTimeline returns an empty Timeline.
func NewTimeline() *Timeline {
	return &Timeline{
		nodes:   make([]Snapshot, 0),
		current: -1,
	}
}

// Add appends a new snapshot and makes it the current snapshot.
func (t *Timeline) Add(label string, data []byte) Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.addLocked(label, data)
}

// Current returns a copy of the current snapshot, or nil if the timeline is empty.
func (t *Timeline) Current() *Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.nodes) == 0 {
		return nil
	}

	snapshot := cloneSnapshot(t.nodes[t.current])
	return &snapshot
}

// Rewind sets the current snapshot to the one with the given ID and returns a copy of it.
func (t *Timeline) Rewind(id string) (*Snapshot, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	index := t.indexOfLocked(id)
	if index < 0 {
		return nil, ErrNotFound
	}

	t.current = index
	snapshot := cloneSnapshot(t.nodes[index])
	return &snapshot, nil
}

// Fork truncates all snapshots after the current one, then adds a new snapshot.
func (t *Timeline) Fork(label string, data []byte) Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.current >= 0 && t.current < len(t.nodes) {
		t.nodes = t.nodes[:t.current+1]
	}

	return t.addLocked(label, data)
}

// List returns copies of all snapshots in the timeline.
func (t *Timeline) List() []Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make([]Snapshot, len(t.nodes))
	for i, snapshot := range t.nodes {
		result[i] = cloneSnapshot(snapshot)
	}
	return result
}

func (t *Timeline) addLocked(label string, data []byte) Snapshot {
	snapshot := Snapshot{
		ID:        nextID(),
		Label:     label,
		Data:      append([]byte(nil), data...),
		CreatedAt: time.Now(),
	}

	t.nodes = append(t.nodes, snapshot)
	t.current = len(t.nodes) - 1
	return snapshot
}

func (t *Timeline) indexOfLocked(id string) int {
	for i, snapshot := range t.nodes {
		if snapshot.ID == id {
			return i
		}
	}
	return -1
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Data = append([]byte(nil), snapshot.Data...)
	return snapshot
}

func nextID() string {
	return fmt.Sprintf("cp-%d-%d", time.Now().UnixNano(), idCounter.Add(1))
}
