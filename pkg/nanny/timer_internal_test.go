package nanny

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"nanny/pkg/notifier"
)

type orderedNotifier struct {
	started      chan struct{}
	release      chan struct{}
	allClearDone chan struct{}
	mu           sync.Mutex
	events       []string
}

func (n *orderedNotifier) Notify(notifier.Message) error {
	close(n.started)
	<-n.release
	n.record("alert")
	return nil
}

func (n *orderedNotifier) NotifyAllClear(notifier.Message) error {
	n.record("all-clear")
	close(n.allClearDone)
	return nil
}

func (n *orderedNotifier) String() string { return "ordered" }

func (n *orderedNotifier) record(event string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, event)
}

func TestExpiryResetAndCallbackAreOrdered(t *testing.T) {
	notif := &orderedNotifier{
		started:      make(chan struct{}),
		release:      make(chan struct{}),
		allClearDone: make(chan struct{}),
	}
	timer := &Timer{
		nanny:      &Nanny{},
		generation: 1,
		timer:      time.NewTimer(time.Hour),
		persist: func(_ Signal, deadline time.Time) {
			if deadline.IsZero() {
				notif.record("remove")
				return
			}
			notif.record("save")
		},
		signal: validSignal{
			Name:       "program",
			Notifier:   notif,
			NextSignal: time.Hour,
			AllClear:   true,
			CallbackFunc: func(*Signal) {
				notif.record("callback")
			},
		},
	}

	expired := make(chan struct{})
	go func() {
		timer.onExpire(1)
		close(expired)
	}()
	<-notif.started

	reset := make(chan struct{})
	go func() {
		timer.resetAfterHeartbeat(timer.signal, timer.persist)
		close(reset)
	}()
	select {
	case <-reset:
	case <-time.After(time.Second):
		t.Fatal("reset blocked on alert delivery")
	}

	close(notif.release)
	<-expired
	select {
	case <-notif.allClearDone:
	case <-time.After(time.Second):
		t.Fatal("all-clear was not delivered after the alert and callback")
	}
	timer.timer.Stop()

	// A callback from the previous generation must be ignored after the reset.
	timer.onExpire(1)
	notif.mu.Lock()
	defer notif.mu.Unlock()
	want := []string{"remove", "save", "alert", "callback", "all-clear"}
	if !reflect.DeepEqual(notif.events, want) {
		t.Fatalf("events = %v, want %v", notif.events, want)
	}
}

type functionNotifier struct {
	notify func(notifier.Message) error
}

func (n *functionNotifier) Notify(message notifier.Message) error {
	return n.notify(message)
}

func (n *functionNotifier) NotifyAllClear(notifier.Message) error { return nil }
func (n *functionNotifier) String() string                        { return "function" }

func TestNotifierCallbackAndErrorHandlerCanReenterHandle(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Nanny, Signal, chan struct{}) Signal
	}{
		{
			name: "notifier",
			setup: func(n *Nanny, replacement Signal, reentered chan struct{}) Signal {
				return Signal{
					Name:       replacement.Name,
					NextSignal: time.Hour,
					Notifier: &functionNotifier{notify: func(notifier.Message) error {
						if err := n.Handle(replacement); err != nil {
							t.Errorf("reentrant Handle() error = %v", err)
						}
						close(reentered)
						return nil
					}},
				}
			},
		},
		{
			name: "callback",
			setup: func(n *Nanny, replacement Signal, reentered chan struct{}) Signal {
				return Signal{
					Name:       replacement.Name,
					Notifier:   replacement.Notifier,
					NextSignal: time.Hour,
					CallbackFunc: func(*Signal) {
						if err := n.Handle(replacement); err != nil {
							t.Errorf("reentrant Handle() error = %v", err)
						}
						close(reentered)
					},
				}
			},
		},
		{
			name: "error handler",
			setup: func(n *Nanny, replacement Signal, reentered chan struct{}) Signal {
				n.ErrorFunc = func(error) {
					if err := n.Handle(replacement); err != nil {
						t.Errorf("reentrant Handle() error = %v", err)
					}
					close(reentered)
				}
				return Signal{
					Name:       replacement.Name,
					NextSignal: time.Hour,
					Notifier: &functionNotifier{notify: func(notifier.Message) error {
						return errors.New("notify failed")
					}},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Nanny{}
			reentered := make(chan struct{})
			replacement := Signal{
				Name:       "program",
				Notifier:   &functionNotifier{notify: func(notifier.Message) error { return nil }},
				NextSignal: time.Hour,
			}
			signal := tt.setup(n, replacement, reentered)
			if err := n.Handle(signal); err != nil {
				t.Fatal(err)
			}
			timer := n.GetTimer(signal.Name)
			timer.lock.Lock()
			generation := timer.generation
			timer.lock.Unlock()
			timer.onExpire(generation)

			select {
			case <-reentered:
			case <-time.After(time.Second):
				t.Fatal("reentrant Handle() deadlocked")
			}
			timer.lock.Lock()
			timer.timer.Stop()
			timer.lock.Unlock()
		})
	}
}

type countingNotifier struct {
	mu    sync.Mutex
	count int
	done  chan struct{}
}

func (n *countingNotifier) Notify(notifier.Message) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.count++
	if n.count == 1 {
		close(n.done)
	}
	return nil
}

func (n *countingNotifier) NotifyAllClear(notifier.Message) error { return nil }
func (n *countingNotifier) String() string                        { return "counting" }

func TestStaleGenerationsAreIgnoredAndExpiryDeliversOnce(t *testing.T) {
	notif := &countingNotifier{done: make(chan struct{})}
	n := &Nanny{}
	signal := Signal{Name: "program", Notifier: notif, NextSignal: time.Hour}
	if err := n.Handle(signal); err != nil {
		t.Fatal(err)
	}
	timer := n.GetTimer(signal.Name)
	timer.lock.Lock()
	staleGeneration := timer.generation
	timer.lock.Unlock()

	if err := n.Handle(signal); err != nil {
		t.Fatal(err)
	}
	timer.lock.Lock()
	currentGeneration := timer.generation
	timer.lock.Unlock()

	timer.onExpire(staleGeneration)
	timer.onExpire(currentGeneration)
	timer.onExpire(currentGeneration)
	select {
	case <-notif.done:
	case <-time.After(time.Second):
		t.Fatal("current generation did not deliver")
	}

	notif.mu.Lock()
	defer notif.mu.Unlock()
	if notif.count != 1 {
		t.Fatalf("Notify() called %d times, want 1", notif.count)
	}
	timer.lock.Lock()
	timer.timer.Stop()
	timer.lock.Unlock()
}

func TestDifferentSignalPersistenceIsIndependent(t *testing.T) {
	n := &Nanny{}
	notif := &functionNotifier{notify: func(notifier.Message) error { return nil }}
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	defer release()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		if err := n.HandleWithPersistence(
			Signal{Name: "first", Notifier: notif, NextSignal: time.Hour},
			func(Signal, time.Time) {
				close(firstStarted)
				<-releaseFirst
			},
		); err != nil {
			t.Errorf("first HandleWithPersistence() error = %v", err)
		}
	}()
	<-firstStarted

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		if err := n.HandleWithPersistence(
			Signal{Name: "second", Notifier: notif, NextSignal: time.Hour},
			func(Signal, time.Time) {},
		); err != nil {
			t.Errorf("second HandleWithPersistence() error = %v", err)
		}
	}()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("one signal's persistence blocked an unrelated signal")
	}

	release()
	<-firstDone
	for _, timer := range n.GetTimers() {
		timer.lock.Lock()
		timer.timer.Stop()
		timer.lock.Unlock()
	}
}

func TestPersistencePanicDoesNotWedgeLaterUpdates(t *testing.T) {
	for _, firstUpdate := range []bool{true, false} {
		name := "reset"
		if firstUpdate {
			name = "initial"
		}
		t.Run(name, func(t *testing.T) {
			n := &Nanny{}
			notif := &functionNotifier{notify: func(notifier.Message) error { return nil }}
			signal := Signal{Name: "program", Notifier: notif, NextSignal: time.Hour}
			if !firstUpdate {
				if err := n.Handle(signal); err != nil {
					t.Fatal(err)
				}
			}

			panicked := false
			func() {
				defer func() { panicked = recover() != nil }()
				_ = n.HandleWithPersistence(signal, func(Signal, time.Time) { panic("persistence failed") })
			}()
			if !panicked {
				t.Fatal("persistence callback did not panic")
			}

			result := make(chan error, 1)
			go func() { result <- n.Handle(signal) }()
			select {
			case err := <-result:
				if err != nil {
					t.Fatalf("Handle() after recovered panic error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Handle() after recovered persistence panic deadlocked")
			}

			timer := n.GetTimer(signal.Name)
			timer.lock.Lock()
			timer.timer.Stop()
			timer.lock.Unlock()
		})
	}
}

type queueBlockingNotifier struct {
	started    chan struct{}
	release    chan struct{}
	once       sync.Once
	deliveries []bool
}

func (n *queueBlockingNotifier) Notify(notifier.Message) error {
	return n.notify(false)
}

func (n *queueBlockingNotifier) NotifyAllClear(notifier.Message) error {
	return n.notify(true)
}

func (n *queueBlockingNotifier) notify(allClear bool) error {
	n.deliveries = append(n.deliveries, allClear)
	blocked := false
	n.once.Do(func() {
		blocked = true
		close(n.started)
	})
	if blocked {
		<-n.release
	}
	return nil
}

func (n *queueBlockingNotifier) String() string { return "queue blocking" }

func TestBlockedNotifierHasBoundedDeliveryQueue(t *testing.T) {
	notif := &queueBlockingNotifier{started: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(notif.release) }) }
	defer release()
	reportedErrors := make(chan error, 1)
	timer := &Timer{
		nanny: &Nanny{ErrorFunc: func(err error) { reportedErrors <- err }},
		signal: validSignal{
			Name:       "program",
			Notifier:   notif,
			NextSignal: time.Hour,
		},
	}
	delivery := timerDelivery{signal: timer.signal}

	timer.lock.Lock()
	timer.alerted = true
	start := timer.enqueueDeliveryLocked(delivery)
	timer.lock.Unlock()
	if !start {
		t.Fatal("first delivery did not start a drainer")
	}
	go timer.drainDeliveries()
	<-notif.started

	for i := range maxPendingDeliveries + 101 {
		allClear := i%2 == 0
		timer.lock.Lock()
		timer.alerted = !allClear
		timer.enqueueDeliveryLocked(timerDelivery{signal: timer.signal, allClear: allClear})
		timer.lock.Unlock()
	}
	timer.lock.Lock()
	queued := len(timer.deliveries)
	overflowed := timer.overflowed
	timer.lock.Unlock()
	if queued != maxPendingDeliveries || !overflowed {
		t.Fatalf("queued=%d overflowed=%t, want queued=%d overflowed=true", queued, overflowed, maxPendingDeliveries)
	}

	release()
	select {
	case err := <-reportedErrors:
		if !strings.Contains(err.Error(), "queue overflow") {
			t.Fatalf("overflow error = %v", err)
		}
		timer.lock.Lock()
		alerted := timer.alerted
		timer.lock.Unlock()
		lastAllClear := notif.deliveries[len(notif.deliveries)-1]
		if lastAllClear == alerted {
			t.Fatalf("last delivery all-clear=%t, timer alerted=%t", lastAllClear, alerted)
		}
	case <-time.After(time.Second):
		t.Fatal("queue overflow was not reported after delivery resumed")
	}
}
