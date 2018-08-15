package nanny_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"nanny/pkg/nanny"
	"nanny/pkg/notifier"
)

// DummyNotifier's only job is to store `msg` argument
// when `Notify()` method is called.
type DummyNotifier struct {
	notifyMsg notifier.Message
	lock      sync.Mutex
}

// Notify stores `msg` in the DummyNotifier.
func (d *DummyNotifier) Notify(msg notifier.Message) error {
	d.lock.Lock()
	d.notifyMsg = msg
	d.lock.Unlock()
	return nil
}

// NotifyMsg retrieves `msg` argument from previous `Notify` call. For testing
// purposes only.
func (d *DummyNotifier) NotifyMsg() notifier.Message {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.notifyMsg
}

// DummyNotifierWithError always returns error.
type DummyNotifierWithError struct{}

// Notify satisfies Notifier interface.
func (d *DummyNotifierWithError) Notify(msg notifier.Message) error {
	return fmt.Errorf("error")
}

func TestNanny(t *testing.T) {
	n := nanny.Nanny{Name: "test nanny"}
	dummy := &DummyNotifier{}
	signal := nanny.Signal{
		Name:       "test program",
		Notifier:   dummy,
		NextSignal: time.Duration(1) * time.Second,
	}

	err := n.Handle(signal)
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}

	// Before the `NextSignal` duration, nothing should happen.
	dummyMsg := dummy.NotifyMsg()
	if dummyMsg.Program != "" {
		t.Errorf("dummy msg should be empty before NextSignal time expires: %v\n", dummyMsg)
	}
	// After 1s, DummyNotifier should return error.
	time.Sleep(time.Duration(1)*time.Second + time.Duration(100)*time.Millisecond)
	dummyMsg = dummy.NotifyMsg()
	if dummyMsg.Program == "" {
		t.Errorf("dummy msg should not be empty after NextSignal time expired: %v\n", dummyMsg)
	}
}

// TestNannyDoesNotNotify tests if no notifier is called when program calls within
// `NextSignal` duration.
func TestNannyDoesNotNotify(t *testing.T) {
	n := nanny.Nanny{}
	dummy := &DummyNotifier{}
	signal := nanny.Signal{
		Name:       "test program",
		Notifier:   dummy,
		NextSignal: time.Duration(1) * time.Second,
	}

	err := n.Handle(signal)
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}

	// Before the `NextSignal` duration, nothing should happen.
	dummyMsg := dummy.NotifyMsg()
	if dummyMsg.Program != "" {
		t.Errorf("dummy msg should be empty before NextSignal time expires: %v\n", dummyMsg)
	}

	// After 0.9s, DummyNotifier should still be empty.
	time.Sleep(time.Duration(900) * time.Millisecond)
	dummyMsg = dummy.NotifyMsg()
	if dummyMsg.Program != "" {
		t.Errorf("dummy msg should be empty before NextSignal time expires: %v\n", dummyMsg)
	}

	// Call handle again to simulate program calling before notification.
	err = n.Handle(signal)
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}

	// DummyNotifier should still be empty.
	dummyMsg = dummy.NotifyMsg()
	if dummyMsg.Program != "" {
		t.Errorf("dummy msg should be empty before NextSignal time expires: %v\n", dummyMsg)
	}
}

func TestEmptyNanny(t *testing.T) {
	n := nanny.Nanny{}
	signal := nanny.Signal{}

	err := n.Handle(signal)
	if err == nil {
		t.Errorf("nanny.Handle should return error when signal.Handler is nil\n")
	}
}

func TestNannyCallsErrorFunc(t *testing.T) {
	var (
		capturedErr error
		// We have to lock access to capturedErr.
		lock sync.Mutex
	)
	errFunc := func(err error) {
		lock.Lock()
		capturedErr = err
		lock.Unlock()
	}
	n := nanny.Nanny{Name: "test nanny", ErrorFunc: errFunc}
	dummy := &DummyNotifierWithError{}
	signal := nanny.Signal{
		Name:       "test program",
		Notifier:   dummy,
		NextSignal: time.Duration(1) * time.Second,
	}

	err := n.Handle(signal)
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}

	time.Sleep(time.Duration(1)*time.Second + time.Duration(100)*time.Millisecond)
	lock.Lock()
	if capturedErr == nil {
		t.Errorf("Nanny did not call ErrorFunc when notify.Notify returned error")
	}
	lock.Unlock()
}

func TestNextSignalZero(t *testing.T) {
	n := nanny.Nanny{}
	signal := nanny.Signal{
		Notifier: &DummyNotifier{},
	}

	err := n.Handle(signal)
	if err == nil {
		t.Errorf("nanny.Handle should return error when signal.Handler is nil\n")
	}
}

func TestConcurrent(t *testing.T) {
	var wg sync.WaitGroup

	n := nanny.Nanny{}

	// Spawn 10 goroutines calling nanny.Handle concurrently. This simulates API
	// being called by multiple callers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			dummy := &DummyNotifier{}
			signal := nanny.Signal{
				Name:       fmt.Sprintf("test program %d", num),
				Notifier:   dummy,
				NextSignal: time.Duration(1) * time.Second,
			}

			err := n.Handle(signal)
			if err != nil {
				t.Errorf("nanny should handle being called from multiple goroutines")
			}

			// Before the `NextSignal` duration, nothing should happen.
			dummyMsg := dummy.NotifyMsg()
			if dummyMsg.Program != "" {
				t.Errorf("dummy msg should be empty before NextSignal time expires: %v\n", dummyMsg)
			}
			// After 1s, DummyNotifier should return error.
			time.Sleep(time.Duration(1)*time.Second + time.Duration(100)*time.Millisecond)
			dummyMsg = dummy.NotifyMsg()
			if dummyMsg.Program == "" {
				t.Errorf("dummy msg should not be empty after NextSignal time expired: %v\n", dummyMsg)
			}
		}(i)
	}

	wg.Wait()
}

func TestMultipleTimerResets(t *testing.T) {
	n := nanny.Nanny{Name: "test nanny"}
	dummy := &DummyNotifier{}
	signal := nanny.Signal{
		Name:       "test program",
		Notifier:   dummy,
		NextSignal: time.Duration(1) * time.Second,
	}

	runHandle := func() {
		err := n.Handle(signal)
		if err != nil {
			t.Errorf("n.Signal should not return error, got: %v\n", err)
		}
	}
	// This would cause data race on timer.Reset.
	for i := 0; i < 100; i++ {
		go runHandle()
	}

	// Before the `NextSignal` duration, nothing should happen.
	dummyMsg := dummy.NotifyMsg()
	if dummyMsg.Program != "" {
		t.Errorf("dummy msg should be empty before NextSignal time expires: %v\n", dummyMsg)
	}
	// After 1s, DummyNotifier should return error.
	time.Sleep(time.Duration(1)*time.Second + time.Duration(100)*time.Millisecond)
	dummyMsg = dummy.NotifyMsg()
	if dummyMsg.Program == "" {
		t.Errorf("dummy msg should not be empty after NextSignal time expired: %v\n", dummyMsg)
	}
}

func TestMsgChange(t *testing.T) {
	n := nanny.Nanny{Name: "test msg changing nanny"}
	dummy := &DummyNotifier{}
	signal := nanny.Signal{
		Name:       "test msg changing program",
		Notifier:   dummy,
		NextSignal: time.Duration(2) * time.Second,
	}

	err := n.Handle(signal)
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}

	// Before the `NextSignal` duration, nothing should happen.
	dummyMsg := dummy.NotifyMsg()
	if dummyMsg.Program != "" {
		t.Errorf("dummy msg should be empty before NextSignal time expires: %v\n", dummyMsg)
	}
	// Call handle with different nextsignal again to simulate program calling before notification.
	err = n.Handle(nanny.Signal{
		Name:       "test msg changing program",
		Notifier:   dummy,
		NextSignal: time.Duration(1) * time.Second,
	})
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}
	// After 1s, DummyNotifier should return error.
	time.Sleep(time.Duration(1)*time.Second + time.Duration(100)*time.Millisecond)
	dummyMsg = dummy.NotifyMsg()
	if dummyMsg.Program == "" {
		t.Errorf("dummy msg should not be empty after NextSignal time expired: %v\n", dummyMsg)
	}

	msg := dummyMsg.Format()
	if strings.Contains(msg, "2s") {
		t.Errorf("dummy msg should not contain 1s after NextSignal time expired: %v\n", dummyMsg)
	}
}

func TestNannyTimer(t *testing.T) {
	n := nanny.Nanny{Name: "test nanny nannyTimer"}
	dummy := &DummyNotifier{}
	signal := nanny.Signal{
		Name:         "test nannyTimer",
		Notifier:     dummy,
		NextSignal:   time.Duration(2) * time.Second,
		CallbackFunc: func(s *nanny.Signal) {},
	}
	err := n.Handle(signal)
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}

	// Trigger the first signal's error.
	time.Sleep(time.Duration(2)*time.Second + time.Duration(100)*time.Millisecond)

	// Before the `NextSignal` duration, nothing should happen.
	dummyMsg := dummy.NotifyMsg()
	if dummyMsg.Program == "" {
		t.Errorf("dummy msg should not be empty after NextSignal time expires: %v\n", dummyMsg)
	}
	// Call handle with different nextsignal again to simulate program calling before notification.
	err = n.Handle(nanny.Signal{
		Name:         "test nannyTimer",
		Notifier:     dummy,
		NextSignal:   time.Duration(1) * time.Second,
		CallbackFunc: func(s *nanny.Signal) {},
	})
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}
	// After 1s, DummyNotifier should return error.
	time.Sleep(time.Duration(1)*time.Second + time.Duration(100)*time.Millisecond)
	dummyMsg = dummy.NotifyMsg()
	if dummyMsg.Program == "" {
		t.Errorf("dummy msg should not be empty after NextSignal time expired: %v\n", dummyMsg)
	}

	msg := dummyMsg.Format()
	if strings.Contains(msg, "2s") {
		t.Errorf("dummy msg should not contain 2s after NextSignal time expired: %v\n", dummyMsg)
	}
}

func TestChangingMeta(t *testing.T) {
	n := nanny.Nanny{Name: "test nanny changing meta"}
	dummy := &DummyNotifier{}
	signal := nanny.Signal{
		Name:         "test changing meta",
		Notifier:     dummy,
		NextSignal:   time.Duration(1) * time.Second,
		CallbackFunc: func(s *nanny.Signal) {},
		Meta: map[string]string{
			"ping": "original-message",
		},
	}
	err := n.Handle(signal)
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}

	// Call handle with different meta again to simulate program calling before notification.
	err = n.Handle(nanny.Signal{
		Name:         "test changing meta",
		Notifier:     dummy,
		NextSignal:   time.Duration(1) * time.Second,
		CallbackFunc: func(s *nanny.Signal) {},
		Meta: map[string]string{
			"ping": "updated-message",
		},
	})
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}
	// After 1s, DummyNotifier should return error.
	time.Sleep(time.Duration(1)*time.Second + time.Duration(100)*time.Millisecond)
	dummyMsg := dummy.NotifyMsg()
	if dummyMsg.Program == "" {
		t.Errorf("dummy msg should not be empty after NextSignal time expired: %v\n", dummyMsg)
	}

	meta := fmt.Sprintf("%v", dummyMsg.Meta)
	if strings.Contains(meta, "original-message") {
		t.Errorf("dummy msg should not contain \"original-message\" after NextSignal time expired: %v\n", dummyMsg)
	}
}
