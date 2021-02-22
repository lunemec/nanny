package nanny

import (
	"encoding/json"
	"math"
	"sync"
	"time"

	"nanny/pkg/notifier"

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

// MarshalJSON marshals a nanny.Timer into JSON. Fields name, notifier, next_signal, all_clear and meta are exported
func (nt *Timer) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name       string            `json:"name"`
		Notifier   string            `json:"notifier"`
		NextSignal string            `json:"next_signal"`
		AllClear   bool              `json:"all_clear"`
		Meta       map[string]string `json:"meta,omitempty"`
	}{
		Name:       nt.signal.Name,
		Notifier:   nt.signal.Notifier.String(),
		NextSignal: nt.end.Format(time.RFC3339),
		AllClear:   nt.signal.AllClear,
		Meta:       nt.signal.Meta,
	})
}

func newTimer(s validSignal, nanny *Nanny) *Timer {
	timer := &Timer{signal: s, nanny: nanny}
	timer.end = time.Now().Add(timer.signal.NextSignal)
	// If NextSignal is in the past but needed for all-clear notification do not notify user until Timer is reset
	if timer.signal.NextSignal.Seconds() > 0 {
		timer.timer = time.AfterFunc(timer.signal.NextSignal, timer.onExpire)
	} else {
		timer.timer = time.AfterFunc(math.MaxInt64, timer.onExpire)
		timer.timer.Stop()
	}
	return timer
}

// Reset updates the nannyTimers signal to reset the timer
func (nt *Timer) Reset(vs validSignal) {
	nt.lock.Lock()
	defer nt.lock.Unlock()

	nt.signal.Notifier = vs.Notifier
	nt.signal.NextSignal = vs.NextSignal
	nt.signal.AllClear = vs.AllClear
	nt.signal.Meta = vs.Meta
	nt.end = time.Now().Add(vs.NextSignal)
	nt.timer.Reset(vs.NextSignal)
}

// ResetAllClear updates the nannyTimers signal to reset the timer
func (nt *Timer) ResetAllClear(vs validSignal) {
	nt.notifyAllClear()

	nt.lock.Lock()
	defer nt.lock.Unlock()

	nt.signal.Notifier = vs.Notifier
	nt.signal.NextSignal = vs.NextSignal
	nt.signal.AllClear = vs.AllClear
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

func (nt *Timer) notifyAllClear() error {
	nt.lock.Lock()
	defer nt.lock.Unlock()
	name := "Nanny"
	if nt.nanny.Name != "" {
		name = nt.nanny.Name
	}

	return nt.signal.Notifier.NotifyAllClear(notifier.Message{
		Nanny:      name,
		Program:    nt.signal.Name,
		NextSignal: nt.signal.NextSignal,
		Meta:       nt.signal.Meta,
	})
}
