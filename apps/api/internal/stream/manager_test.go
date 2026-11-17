package stream

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestManager(queueSize int) *Manager {
	return NewManager(queueSize, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestSubscribeAndPublish(t *testing.T) {
	m := newTestManager(4)
	envID := uuid.New()
	keyID := uuid.New()

	_, events, err := m.Subscribe(context.Background(), envID, keyID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	m.Publish(envID, 7)

	select {
	case ev := <-events:
		if ev.Version != 7 || ev.ID != 7 {
			t.Errorf("event = %+v, want version and id 7", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestPublishOnlyReachesMatchingEnvironment(t *testing.T) {
	m := newTestManager(4)
	envA, envB := uuid.New(), uuid.New()
	keyID := uuid.New()

	_, eventsA, _ := m.Subscribe(context.Background(), envA, keyID)
	_, eventsB, _ := m.Subscribe(context.Background(), envB, keyID)

	m.Publish(envA, 3)

	select {
	case ev := <-eventsA:
		if ev.Version != 3 {
			t.Errorf("event version = %d, want 3", ev.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("environment A received no event")
	}

	select {
	case ev := <-eventsB:
		t.Errorf("environment B received an event it should not have: %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestPublishDoesNotBlockOnSlowSubscriber is the backpressure guarantee.
// A subscriber that never reads must not delay the publisher.
func TestPublishDoesNotBlockOnSlowSubscriber(t *testing.T) {
	m := newTestManager(2)
	envID := uuid.New()
	keyID := uuid.New()

	// One subscriber that never reads its channel.
	_, _, _ = m.Subscribe(context.Background(), envID, keyID)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Publish far more than the queue can hold.
		for i := 1; i <= 100; i++ {
			m.Publish(envID, int64(i))
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}

	// The slow subscriber is not dropped. Its pending event is replaced
	// with the newest version, which is all it needs: it fetches the full
	// snapshot on any version it sees.
	if m.ActiveCount() != 1 {
		t.Errorf("active count = %d, want 1", m.ActiveCount())
	}
}

// TestPublishingSubscriberKeepsItsConnection checks that a subscriber
// which reads its events is not dropped, even under a burst.
func TestPublishingSubscriberKeepsItsConnection(t *testing.T) {
	m := newTestManager(16)
	envID := uuid.New()
	keyID := uuid.New()

	sub, events, _ := m.Subscribe(context.Background(), envID, keyID)

	// Drain continuously so the queue never fills.
	received := make(chan int, 256)
	go func() {
		for ev := range events {
			received <- int(ev.Version)
		}
	}()

	for i := 1; i <= 50; i++ {
		m.Publish(envID, int64(i))
	}

	// Give the reader a moment to drain.
	time.Sleep(200 * time.Millisecond)

	if m.ActiveCount() != 1 {
		t.Errorf("active count = %d, want 1; a reading subscriber was dropped", m.ActiveCount())
	}

	select {
	case v := <-received:
		if v < 1 {
			t.Errorf("received version %d, want at least 1", v)
		}
	default:
		t.Error("the reading subscriber received no events")
	}

	m.Unsubscribe(envID, sub.ID())
}

// TestSlowSubscriberKeepsLatestVersion checks that a subscriber which
// falls behind keeps only the newest pending version.
//
// Intermediate versions carry no information the subscriber needs: it
// fetches the full snapshot when it sees any newer version, so dropping
// the connection would be unnecessary disruption.
func TestSlowSubscriberKeepsLatestVersion(t *testing.T) {
	m := newTestManager(1)
	envID := uuid.New()
	keyID := uuid.New()

	_, events, _ := m.Subscribe(context.Background(), envID, keyID)

	// Publish several versions without reading.
	for i := 1; i <= 5; i++ {
		m.Publish(envID, int64(i))
	}

	// The subscriber must still be connected.
	if m.ActiveCount() != 1 {
		t.Errorf("active count = %d, want 1", m.ActiveCount())
	}

	// The pending event must be the newest version.
	select {
	case ev := <-events:
		if ev.Version != 5 {
			t.Errorf("pending version = %d, want 5", ev.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("no pending event")
	}
}

func TestRevokeKeyClosesStreams(t *testing.T) {
	m := newTestManager(4)
	envID := uuid.New()
	keyID := uuid.New()
	otherKey := uuid.New()

	sub, _, _ := m.Subscribe(context.Background(), envID, keyID)
	otherSub, _, _ := m.Subscribe(context.Background(), envID, otherKey)

	closed := m.RevokeKey(keyID)
	if closed != 1 {
		t.Errorf("RevokeKey closed %d streams, want 1", closed)
	}

	select {
	case <-sub.Closed():
	case <-time.After(time.Second):
		t.Error("the revoked subscriber's stream was not closed")
	}

	// Another credential's stream must be untouched.
	select {
	case <-otherSub.Closed():
		t.Error("a different credential's stream was closed")
	default:
	}

	if m.ActiveCount() != 1 {
		t.Errorf("active count = %d, want 1", m.ActiveCount())
	}
}

func TestConnectionLimitTracking(t *testing.T) {
	m := newTestManager(4)
	envID := uuid.New()
	keyID := uuid.New()

	_, _, _ = m.Subscribe(context.Background(), envID, keyID)
	_, _, _ = m.Subscribe(context.Background(), envID, keyID)

	if got := m.ConnectionCount(keyID); got != 2 {
		t.Errorf("connection count = %d, want 2", got)
	}
	if got := m.ActiveCount(); got != 2 {
		t.Errorf("active count = %d, want 2", got)
	}
}

func TestUnsubscribeRemovesSubscriber(t *testing.T) {
	m := newTestManager(4)
	envID := uuid.New()
	keyID := uuid.New()

	sub, _, _ := m.Subscribe(context.Background(), envID, keyID)
	m.Unsubscribe(envID, sub.ID())

	if m.ActiveCount() != 0 {
		t.Errorf("active count = %d, want 0", m.ActiveCount())
	}
	if m.ConnectionCount(keyID) != 0 {
		t.Errorf("connection count = %d, want 0", m.ConnectionCount(keyID))
	}
}

func TestCloseClosesEverything(t *testing.T) {
	m := newTestManager(4)
	envID := uuid.New()
	keyID := uuid.New()

	sub, _, _ := m.Subscribe(context.Background(), envID, keyID)
	m.Close()

	select {
	case <-sub.Closed():
	case <-time.After(time.Second):
		t.Error("Close did not close the subscriber")
	}

	if _, _, err := m.Subscribe(context.Background(), envID, keyID); err == nil {
		t.Error("Subscribe succeeded after Close")
	}
}

// TestConcurrentPublishAndSubscribe exercises the manager's shared state
// so that the race detector covers it.
func TestConcurrentPublishAndSubscribe(t *testing.T) {
	m := newTestManager(8)
	envID := uuid.New()
	keyID := uuid.New()

	var wg sync.WaitGroup

	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				sub, events, err := m.Subscribe(context.Background(), envID, keyID)
				if err != nil {
					return
				}
				// Drain so the subscriber does not get dropped.
				go func() {
					for range events {
					}
				}()
				m.Publish(envID, int64(i))
				m.Unsubscribe(envID, sub.ID())
			}
		}()
	}

	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				m.Publish(envID, int64(i))
				_ = m.ActiveCount()
				_ = m.EnvironmentCount()
			}
		}()
	}

	wg.Wait()
}
