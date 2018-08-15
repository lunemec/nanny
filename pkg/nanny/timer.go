package nanny

import (
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

	lock sync.Mutex
}

func newTimer(s validSignal, nanny *Nanny) *Timer {
	timer := &Timer{signal: s, nanny: nanny}
	timer.timer = time.AfterFunc(timer.signal.NextSignal, timer.onExpire)
	return timer
}

// Reset updates the nannyTimers signal to reset the timer
func (nt *Timer) Reset(vs validSignal) {
	nt.lock.Lock()
	defer nt.lock.Unlock()

	nt.signal.NextSignal = vs.NextSignal
	nt.signal.Meta = vs.Meta
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
