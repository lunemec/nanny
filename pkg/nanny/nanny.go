package nanny

import (
	"fmt"
	"sync"
	"time"

	"nanny/pkg/notifier"

	"github.com/cornelk/hashmap"
	"github.com/pkg/errors"
)

// Nanny represents the main functionality with its `Handle` func.
// There should be only one nanny per process.
type Nanny struct {
	Name string // Nanny's name.
	// Function that will be called when notifier.Notify returns error.
	// If not specified, uses defaultErrorFunc.
	ErrorFunc ErrorFunc
	timers    hashmap.HashMap // Map of program names (Signal.Name) to their timers.

	lock sync.Mutex
}

// Signal represents program calling nanny to notify with given notifier if
// this program does not call again within NextSignal + MaxDeviation.
type Signal struct {
	// Name of program being monitored.
	// Should be unique for each instance of a program.
	Name       string
	Notifier   notifier.Notifier // What notifier to use.
	NextSignal time.Duration     // Notify after reaching this timeout.
	Meta       map[string]string

	// Optional callback function that will be called when notifier is called.
	CallbackFunc func(*Signal)
}

// validSignal represents signal that is actually valid. It is created by calling
// nanny.validate(Signal) internally.
type validSignal Signal

// ErrorFunc is a function that will be called by Nanny if there was any error
// during notifier.Notify call.
type ErrorFunc func(error)

// NannyTimer encapsulates a signal and its timer
type nannyTimer struct {
	signal validSignal
	timer  *time.Timer
}

// Reset updates the nannyTimers signal to reset the timer
func (n *nannyTimer) Reset(d time.Duration) {
	n.signal.NextSignal = d
	n.timer.Reset(d)
}

// defaultErrorFunc is used when no Nanny.ErrorFunc is specified, it simply prints
// the error to stdout.
func defaultErrorFunc(err error) {
	fmt.Println(err)
}

// Handle creates new timer within `Nanny`, which calls `signal.Notifier.Notify()` if there is no
// signal within NextSignal + MaxDeviation.
func (n *Nanny) Handle(s Signal) error {
	vs, err := n.validate(s)
	if err != nil {
		return errors.Wrap(err, "signal is invalid")
	}

	return n.handle(vs)
}

// validate does simple sanity check.
func (n *Nanny) validate(s Signal) (validSignal, error) {
	var vs validSignal

	if s.Notifier == nil {
		return vs, errors.New("signal.Handler is nil")
	}

	if s.NextSignal == 0 {
		return vs, errors.New("signal.NextSignal cannot be 0")
	}

	return validSignal(s), nil
}

// handle is called only when signal has been successfully validated.
func (n *Nanny) handle(s validSignal) error {
	// Check if this program already has goroutine that needs cancelling.
	timer := n.GetTimer(s.Name)

	if timer != nil {
		// Timer exists, reset the timer to the new signal value.
		n.lock.Lock()
		timer.Reset(s.NextSignal)
		n.lock.Unlock()
	} else {
		// No timer is registered for this program, create it.
		newTimer := &nannyTimer{signal: s}
		newTimer.timer = time.AfterFunc(newTimer.signal.NextSignal, func() {
			err := newTimer.signal.Notifier.Notify(n.msg(newTimer.signal))
			if err != nil {
				// Add context to the error message and call ErrorFunc.
				err = errors.Wrapf(err, "error calling notifier: %T with signal: %+v", newTimer.signal.Notifier, newTimer.signal)
				if n.ErrorFunc == nil {
					defaultErrorFunc(err)
				} else {
					n.ErrorFunc(err)
				}
			}

			// Call callback if set.
			if s.CallbackFunc != nil {
				signal := Signal(newTimer.signal)
				newTimer.signal.CallbackFunc(&signal)
			}
		})
		n.SetTimer(s.Name, newTimer)
	}

	return nil
}

// msg creates message from validSignal that will be sent via notifier.
func (n *Nanny) msg(s validSignal) notifier.Message {
	name := "Nanny"
	if n.Name != "" {
		name = n.Name
	}
	return notifier.Message{
		Nanny:      name,
		Program:    s.Name,
		NextSignal: s.NextSignal,
		Meta:       s.Meta,
	}
}

// GetTimer returns time.Timer when given program name is already registered or
// nil.
func (n *Nanny) GetTimer(name string) *nannyTimer {
	value, ok := n.timers.GetStringKey(name)
	if !ok {
		return nil
	}
	return value.(*nannyTimer)
}

// SetTimer sets new timer for given program name.
func (n *Nanny) SetTimer(name string, timer *nannyTimer) {
	n.timers.Set(name, timer)
}
