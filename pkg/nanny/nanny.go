package nanny

import (
	"fmt"
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
		timer.Reset(s)
	} else {
		// No timer is registered for this program, create it.
		n.SetTimer(s.Name, newTimer(s, n))
	}

	return nil
}

// GetTimer returns time.Timer when given program name is already registered or
// nil.
func (n *Nanny) GetTimer(name string) *Timer {
	value, ok := n.timers.GetStringKey(name)
	if !ok {
		return nil
	}
	return value.(*Timer)
}

// SetTimer sets new timer for given program name.
func (n *Nanny) SetTimer(name string, timer *Timer) {
	n.timers.Set(name, timer)
}

// GetTimers returns a slice of currently open timers
func (n *Nanny) GetTimers() []*Timer {
	timers := make([]*Timer, n.timers.Len())
	index := 0
	for timer := range n.timers.Iter() {
		timers[index] = timer.Value.(*Timer)
		index = index + 1
	}
	return timers
}
