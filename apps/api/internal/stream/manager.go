// Package stream implements the Server-Sent Events manager.
//
// SSE is notification delivery only. An event carries the environment's
// configuration version; the SDK fetches the authoritative snapshot from
// the config endpoint.
//
// The manager never blocks a flag update. A subscriber whose queue is
// full is disconnected, and the SDK reconnects and fetches the latest
// snapshot.
package stream

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// Event is a configuration notification.
type Event struct {
	// ID is the environment's configuration version. It is also written
	// to the SSE id field, so Last-Event-ID is interpretable as a version.
	ID      int64
	Version int64
}

// Subscriber is one connected client.
type Subscriber struct {
	id        uuid.UUID
	envID     uuid.UUID
	keyID     uuid.UUID
	events    chan Event
	closeOnce sync.Once
	closed    chan struct{}
}

// close signals the subscriber's writer goroutine to stop.
func (s *Subscriber) close() {
	s.closeOnce.Do(func() {
		close(s.closed)
	})
}

// Closed returns a channel that is closed when the subscriber is
// disconnected, for example by a key revocation.
func (s *Subscriber) Closed() <-chan struct{} {
	return s.closed
}

// replacePending replaces the oldest queued event with a newer one.
//
// It returns false when the queue is empty, because a concurrent reader
// has already taken the event and there is nothing to replace.
//
// The replacement is safe with a concurrent reader: the reader may take
// either the old or the new event, and both describe a version the
// subscriber will act on by fetching the latest snapshot.
func (s *Subscriber) replacePending(event Event) bool {
	// Drain the oldest event, then put the newer one back.
	select {
	case <-s.events:
	default:
		// Nothing queued. A reader took it, or the queue was emptied by a
		// concurrent unsubscribe.
		return false
	}

	select {
	case s.events <- event:
		return true
	default:
		// The reader refilled the queue in the meantime. The event was
		// dropped, but the reader has newer information anyway.
		return false
	}
}

// ID returns the subscriber's identifier.
func (s *Subscriber) ID() uuid.UUID {
	return s.id
}

// Manager maintains the subscriber registry.
type Manager struct {
	mu          sync.RWMutex
	subscribers map[uuid.UUID]map[uuid.UUID]*Subscriber
	queueSize   int
	logger      *slog.Logger

	// activeCount tracks connections per credential for the connection
	// limit. It is in memory; no external store is involved.
	perKeyCount map[uuid.UUID]int

	closed atomic.Bool
}

// NewManager returns a manager with the given per-subscriber queue size.
func NewManager(queueSize int, logger *slog.Logger) *Manager {
	if queueSize < 1 {
		queueSize = 1
	}
	return &Manager{
		subscribers: make(map[uuid.UUID]map[uuid.UUID]*Subscriber),
		queueSize:   queueSize,
		logger:      logger,
		perKeyCount: make(map[uuid.UUID]int),
	}
}

// Subscribe registers a subscriber for an environment.
//
// It returns the subscriber and the channel to read events from. The
// caller must call Unsubscribe when the connection ends.
func (m *Manager) Subscribe(ctx context.Context, envID, keyID uuid.UUID) (*Subscriber, <-chan Event, error) {
	if m.closed.Load() {
		return nil, nil, context.Canceled
	}

	sub := &Subscriber{
		id:     uuid.New(),
		envID:  envID,
		keyID:  keyID,
		events: make(chan Event, m.queueSize),
		closed: make(chan struct{}),
	}

	m.mu.Lock()
	if m.subscribers[envID] == nil {
		m.subscribers[envID] = make(map[uuid.UUID]*Subscriber)
	}
	m.subscribers[envID][sub.id] = sub
	m.perKeyCount[keyID]++
	m.mu.Unlock()

	return sub, sub.events, nil
}

// Unsubscribe removes a subscriber.
func (m *Manager) Unsubscribe(envID, subID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	subs, ok := m.subscribers[envID]
	if !ok {
		return
	}
	sub, ok := subs[subID]
	if !ok {
		return
	}
	delete(subs, subID)
	if len(subs) == 0 {
		delete(m.subscribers, envID)
	}
	m.perKeyCount[sub.keyID]--
	if m.perKeyCount[sub.keyID] <= 0 {
		delete(m.perKeyCount, sub.keyID)
	}
	sub.close()
}

// Publish sends a version notification to every subscriber of an
// environment.
//
// It never blocks. If a subscriber's queue is full, that subscriber is
// disconnected so it reconnects and fetches the latest snapshot. A slow
// client must never delay a flag update or another subscriber.
func (m *Manager) Publish(envID uuid.UUID, version int64) {
	m.mu.RLock()
	subs, ok := m.subscribers[envID]
	if !ok {
		m.mu.RUnlock()
		return
	}

	// Copy the subscriber list under the read lock, then send outside it.
	// Sending under the lock would let one slow subscriber block the
	// registry for everyone.
	list := make([]*Subscriber, 0, len(subs))
	for _, s := range subs {
		list = append(list, s)
	}
	m.mu.RUnlock()

	event := Event{ID: version, Version: version}

	var dropped []*Subscriber
	for _, s := range list {
		select {
		case s.events <- event:
		default:
			// The queue is full. The subscriber is behind.
			//
			// An intermediate version carries no information the subscriber
			// does not already have: it fetches the full snapshot and only
			// needs the newest version. So the pending event is replaced
			// with the newer one rather than the subscriber being
			// disconnected.
			//
			// If the queue is full of events that cannot be replaced,
			// because the reader is consuming them concurrently, the
			// subscriber is disconnected instead. It reconnects and fetches
			// the latest snapshot, which recovers any missed events.
			if s.replacePending(event) {
				continue
			}
			dropped = append(dropped, s)
		}
	}

	for _, s := range dropped {
		m.logger.Warn("dropping slow sse subscriber",
			"subscriber_id", s.id,
			"environment_id", envID,
			"key_id", s.keyID,
			"version", version,
		)
		m.Unsubscribe(envID, s.id)
	}
}

// RevokeKey closes every stream belonging to a credential.
//
// Revocation is immediate on a single instance, so a revoked key cannot
// keep an established stream open.
func (m *Manager) RevokeKey(keyID uuid.UUID) int {
	m.mu.Lock()
	var toClose []*Subscriber
	for envID, subs := range m.subscribers {
		for _, s := range subs {
			if s.keyID == keyID {
				toClose = append(toClose, s)
				delete(subs, s.id)
			}
		}
		if len(subs) == 0 {
			delete(m.subscribers, envID)
		}
	}
	delete(m.perKeyCount, keyID)
	m.mu.Unlock()

	for _, s := range toClose {
		s.close()
	}
	return len(toClose)
}

// ConnectionCount returns the number of active connections for a
// credential.
func (m *Manager) ConnectionCount(keyID uuid.UUID) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.perKeyCount[keyID]
}

// ActiveCount returns the total number of active connections.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := 0
	for _, subs := range m.subscribers {
		total += len(subs)
	}
	return total
}

// EnvironmentCount returns the number of environments with subscribers.
func (m *Manager) EnvironmentCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.subscribers)
}

// Close closes every subscriber and stops the manager accepting new ones.
func (m *Manager) Close() {
	m.closed.Store(true)

	m.mu.Lock()
	var toClose []*Subscriber
	for envID, subs := range m.subscribers {
		for _, s := range subs {
			toClose = append(toClose, s)
		}
		delete(m.subscribers, envID)
	}
	m.perKeyCount = make(map[uuid.UUID]int)
	m.mu.Unlock()

	for _, s := range toClose {
		s.close()
	}
}
