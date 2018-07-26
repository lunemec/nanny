package nanny

import (
	"nanny/pkg/notifier"
	"time"

	"github.com/pkg/errors"
)

// NannyTimer encapsulates a signal and its timer
type nannyTimer struct {
	signal validSignal
	timer  *time.Timer
	nanny  *Nanny

	errorFunc ErrorFunc
}

func newNannyTimer(s validSignal, nanny *Nanny) *nannyTimer {
	timer := &nannyTimer{signal: s, nanny: nanny}
	timer.timer = time.AfterFunc(timer.signal.NextSignal, timer.onExpire)
	return timer
}

// Reset updates the nannyTimers signal to reset the timer
func (nt *nannyTimer) Reset(d time.Duration) {
	nt.nanny.lock.Lock()
	nt.signal.NextSignal = d
	nt.timer.Reset(d)
	nt.nanny.lock.Unlock()
}

func (nt *nannyTimer) onExpire() {
	err := nt.notify()
	if err != nil {
		// Add context to the error message and call ErrorFunc.
		err = errors.Wrapf(err, "error calling notifier: %T with signal: %+v", nt.signal.Notifier, nt.signal)
		if nt.errorFunc == nil {
			defaultErrorFunc(err)
		} else {
			nt.errorFunc(err)
		}
	}

	// Call callback if set.
	if nt.signal.CallbackFunc != nil {
		signal := Signal(nt.signal)
		nt.signal.CallbackFunc(&signal)
	}
}

func (nt *nannyTimer) notify() error {
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
