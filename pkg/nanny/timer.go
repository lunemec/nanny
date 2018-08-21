package nanny

import (
	"encoding/json"
	"nanny/pkg/notifier"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// Timer encapsulates a signal and its timer
type Timer struct {
	signal validSignal
	timer  *time.Timer
	nanny  *Nanny
	end    time.Time

	lock sync.Mutex
}

// MarshalJSON marshals a nanny.Timer into JSON. Fields name, notifier, next_signal and meta are exported
func (nt *Timer) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name       string            `json:"name"`
		Notifier   string            `json:"notifier"`
		NextSignal string            `json:"next_signal"`
		Meta       map[string]string `json:"meta,omitempty"`
	}{
		Name:       nt.signal.Name,
		Notifier:   nt.signal.Notifier.String(),
		NextSignal: nt.end.Format(time.RFC3339),
		Meta:       nt.signal.Meta,
	})
}

func newTimer(s validSignal, nanny *Nanny) *Timer {
	timer := &Timer{signal: s, nanny: nanny}
	timer.end = time.Now().Add(timer.signal.NextSignal)
	timer.timer = time.AfterFunc(timer.signal.NextSignal, timer.onExpire)
	return timer
}

// Reset updates the nannyTimers signal to reset the timer
func (nt *Timer) Reset(vs validSignal) {
	nt.lock.Lock()
	defer nt.lock.Unlock()

	nt.signal.NextSignal = vs.NextSignal
	nt.signal.Meta = vs.Meta
	nt.end = time.Now().Add(vs.NextSignal)
	nt.timer.Reset(vs.NextSignal)
}

func (nt *Timer) onExpire() {
	err := nt.notify()
	if err != nil {
		// Add context to the error message and call ErrorFunc.
		err = errors.Wrapf(err, "error calling notifier: %T with signal: %+v", nt.signal.Notifier, nt.signal)
		if nt.nanny.ErrorFunc == nil {
			defaultErrorFunc(err)
		} else {
			nt.nanny.ErrorFunc(err)
		}
	}

	// Call callback if set.
	nt.lock.Lock()
	if nt.signal.CallbackFunc != nil {
		signal := Signal(nt.signal)
		nt.signal.CallbackFunc(&signal)
	}
	nt.lock.Unlock()
}

func (nt *Timer) notify() error {
	nt.lock.Lock()
	defer nt.lock.Unlock()
	name := "Nanny"
	if nt.nanny.Name != "" {
		name = nt.nanny.Name
	}

	return nt.signal.Notifier.Notify(notifier.Message{
		Nanny:      name,
		Program:    nt.signal.Name,
		NextSignal: nt.signal.NextSignal,
		Meta:       nt.signal.Meta,
	})
}
