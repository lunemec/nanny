package nanny

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"nanny/pkg/notifier"
)

type orderedNotifier struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	events  []string
}

func (n *orderedNotifier) Notify(notifier.Message) error {
	close(n.started)
	<-n.release
	n.record("alert")
	return nil
}

func (n *orderedNotifier) NotifyAllClear(notifier.Message) error {
	n.record("all-clear")
	return nil
}

func (n *orderedNotifier) String() string { return "ordered" }

func (n *orderedNotifier) record(event string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, event)
}

func TestExpiryResetAndCallbackAreOrdered(t *testing.T) {
	notif := &orderedNotifier{started: make(chan struct{}), release: make(chan struct{})}
	timer := &Timer{
		nanny:      &Nanny{},
		generation: 1,
		timer:      time.NewTimer(time.Hour),
		signal: validSignal{
			Name:       "program",
			Notifier:   notif,
			NextSignal: time.Hour,
			AllClear:   true,
			CallbackFunc: func(*Signal) {
				notif.record("remove")
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
		timer.resetAfterHeartbeat(timer.signal)
		close(reset)
	}()
	select {
	case <-reset:
		t.Fatal("reset completed while alert delivery was still running")
	case <-time.After(20 * time.Millisecond):
	}

	close(notif.release)
	<-expired
	<-reset
	timer.timer.Stop()

	// A callback from the previous generation must be ignored after the reset.
	timer.onExpire(1)
	notif.mu.Lock()
	defer notif.mu.Unlock()
	want := []string{"alert", "remove", "all-clear"}
	if !reflect.DeepEqual(notif.events, want) {
		t.Fatalf("events = %v, want %v", notif.events, want)
	}
}
