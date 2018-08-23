package nanny_test

import (
	"encoding/json"
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

func (d *DummyNotifier) String() string {
	return "dummy"
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

func (d *DummyNotifierWithError) String() string {
	return "dummy with error"
}

func createTimer(nannyName, signalName string, duration time.Duration, meta map[string]string) *nanny.Timer {
	n := nanny.Nanny{Name: nannyName}
	dummy := &DummyNotifier{}
	err := n.Handle(nanny.Signal{
		Name:         signalName,
		Notifier:     dummy,
		NextSignal:   duration,
		CallbackFunc: func(s *nanny.Signal) {},
		Meta:         meta,
	})
	if err != nil {
		return nil
	}
	return n.GetTimer(signalName)
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

func TestGetTimers(t *testing.T) {
	n := nanny.Nanny{Name: "test nanny GetTimers"}
	dummy := &DummyNotifier{}
	err := n.Handle(nanny.Signal{
		Name:         "test signal 1",
		Notifier:     dummy,
		NextSignal:   time.Duration(1) * time.Second,
		CallbackFunc: func(s *nanny.Signal) {},
		Meta:         map[string]string{},
	})
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}
	err = n.Handle(nanny.Signal{
		Name:         "test signal 2",
		Notifier:     dummy,
		NextSignal:   time.Duration(1) * time.Hour,
		CallbackFunc: func(s *nanny.Signal) {},
		Meta:         map[string]string{},
	})
	if err != nil {
		t.Errorf("n.Signal should not return error, got: %v\n", err)
	}
	timers := n.GetTimers()
	if len(timers) != 2 {
		t.Errorf("n.GetTimers should return 2 timers, got: %v\n", len(timers))
	}
}
func TestTimerMarshalJSONName(t *testing.T) {
	name := "test MarshalJSON name"
	timer := createTimer(name, name, time.Second, map[string]string{})
	if timer == nil {
		t.Error("expected timer, got nil")
	}

	jsonBytes, err := json.Marshal(timer)
	if err != nil {
		t.Errorf("json.Marshal should not return error, got: %v\n", err)
	}

	if !strings.Contains(string(jsonBytes), name) {
		t.Error("expected json representation to contain the signals name")
	}
}

func TestTimerMarshalJSONMetaNotPresent(t *testing.T) {
	name := "test MarshalJSON meta not present"
	timer := createTimer(name, name, time.Second, map[string]string{})
	if timer == nil {
		t.Error("expected timer, got nil")
	}

	jsonBytes, err := json.Marshal(timer)
	if err != nil {
		t.Errorf("json.Marshal should not return error, got: %v\n", err)
	}

	if strings.Contains(string(jsonBytes), "\"meta\":") {
		t.Error("expected json representation to not contain \"meta\"")
	}
}

func TestTimerMarshalJSONMeta(t *testing.T) {
	name := "test MarshalJSON meta"
	timer := createTimer(name, name, time.Second, map[string]string{
		"key": "value",
	})
	if timer == nil {
		t.Error("expected timer, got nil")
	}

	jsonBytes, err := json.Marshal(timer)
	if err != nil {
		t.Errorf("json.Marshal should not return error, got: %v\n", err)
	}
	jsonString := string(jsonBytes)

	for _, required := range []string{"\"meta\":", "\"key\":\"value\""} {
		if !strings.Contains(jsonString, required) {
			t.Errorf("expected json representation to contain \"%s\"", required)
		}
	}
}

func TestTimerMarshalJSONNextSignal(t *testing.T) {
	var jsonSignal struct {
		NextSignal string `json:"next_signal"`
	}

	// Test setup
	n := nanny.Nanny{Name: "test timer MarshalJSON next signal"}
	dummy := &DummyNotifier{}

	signalName := "test_signal_json_next_signal"
	dur := time.Duration(3) * time.Second
	expectedEnd := time.Now().Add(dur)

	n.Handle(nanny.Signal{
		Name:       signalName,
		Notifier:   dummy,
		NextSignal: dur,
	})

	timer := n.GetTimer(signalName)

	jsonBytes, _ := json.Marshal(timer)

	// Unmarshal the jsonBytes
	_ = json.Unmarshal(jsonBytes, &jsonSignal)
	parsedNextSignal, err := time.Parse(time.RFC3339, jsonSignal.NextSignal)
	if err != nil {
		t.Errorf("Expected time.Parse without error but got error, got: %v\n", err)
	}

	diff := expectedEnd.Sub(parsedNextSignal)

	if diff > time.Second {
		t.Errorf("Expected next_signal in json to be less than 1s, got: %v\n", diff)
	}

	// Sleep some time to check subsequent calls
	time.Sleep(time.Duration(2) * time.Second)

	expectedEnd = time.Now().Add(dur)

	n.Handle(nanny.Signal{
		Name:       signalName,
		Notifier:   dummy,
		NextSignal: dur,
	})
	timer = n.GetTimer(signalName)

	jsonBytes, _ = json.Marshal(timer)

	// Unmarshal the jsonBytes
	json.Unmarshal(jsonBytes, &jsonSignal)
	parsedNextSignal, err = time.Parse(time.RFC3339, jsonSignal.NextSignal)
	if err != nil {
		t.Errorf("Expected time.Parse without error but got error, got: %v\n", err)
	}

	// check if the returned next_signal is not stale
	diff = expectedEnd.Sub(parsedNextSignal)

	if diff > time.Second {
		t.Errorf("Expected next_signal in json to be less than 1s, got: %v\n", diff)
	}
}
