package nanny

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"nanny/pkg/notifier"
)

// Timer encapsulates a signal and its timer
type Timer struct {
	signal     validSignal
	timer      *time.Timer
	nanny      *Nanny
	end        time.Time
	generation uint64
	alerted    bool

	lock sync.Mutex
}

// MarshalJSON marshals a nanny.Timer into JSON. Fields name, notifier, next_signal, all_clear and meta are exported
func (nt *Timer) MarshalJSON() ([]byte, error) {
	nt.lock.Lock()
	defer nt.lock.Unlock()

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
	timer := &Timer{signal: s, nanny: nanny, generation: 1}
	timer.end = time.Now().Add(timer.signal.NextSignal)
	timer.scheduleLocked()
	return timer
}

// Reset updates the nannyTimers signal to reset the timer
func (nt *Timer) Reset(vs validSignal) {
	nt.reset(vs, false, false)
}

func (nt *Timer) resetAfterHeartbeat(vs validSignal) {
	nt.reset(vs, vs.AllClear, false)
}

func (nt *Timer) reset(vs validSignal, sendAllClear, forceAllClear bool) {
	nt.lock.Lock()

	var notifyErr error
	if sendAllClear && (forceAllClear || nt.alerted) {
		if err := nt.notifyAllClearLocked(); err != nil {
			notifyErr = nt.wrapNotifyErrorLocked(err)
		}
	}
	nt.timer.Stop()
	nt.signal = vs
	nt.end = time.Now().Add(nt.signal.NextSignal)
	nt.generation++
	nt.alerted = false
	nt.scheduleLocked()
	nt.lock.Unlock()

	if notifyErr != nil {
		nt.reportNotifyError(notifyErr)
	}
}

// ResetAllClear updates the nannyTimers signal to reset the timer
func (nt *Timer) ResetAllClear(vs validSignal) {
	nt.reset(vs, true, true)
}

func (nt *Timer) scheduleLocked() {
	if nt.signal.NextSignal <= 0 {
		nt.alerted = true
		nt.timer = time.AfterFunc(math.MaxInt64, func() {})
		nt.timer.Stop()
		return
	}
	generation := nt.generation
	nt.timer = time.AfterFunc(nt.signal.NextSignal, func() {
		nt.onExpire(generation)
	})
}

func (nt *Timer) onExpire(generation uint64) {
	nt.lock.Lock()

	if generation != nt.generation || nt.alerted {
		nt.lock.Unlock()
		return
	}
	nt.alerted = true
	var notifyErr error
	if err := nt.notifyLocked(); err != nil {
		notifyErr = nt.wrapNotifyErrorLocked(err)
	}

	// Call callback if set.
	if nt.signal.CallbackFunc != nil {
		signal := Signal(nt.signal)
		nt.signal.CallbackFunc(&signal)
	}
	nt.lock.Unlock()

	if notifyErr != nil {
		nt.reportNotifyError(notifyErr)
	}
}

func (nt *Timer) wrapNotifyErrorLocked(err error) error {
	return fmt.Errorf("error calling notifier %T with signal %+v: %w", nt.signal.Notifier, nt.signal, err)
}

func (nt *Timer) reportNotifyError(err error) {
	if nt.nanny.ErrorFunc == nil {
		defaultErrorFunc(err)
		return
	}
	nt.nanny.ErrorFunc(err)
}

func (nt *Timer) notifyLocked() error {
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

func (nt *Timer) notifyAllClearLocked() error {
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
