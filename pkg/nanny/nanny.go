package nanny

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"nanny/pkg/notifier"
)

// Nanny represents the main functionality with its `Handle` func.
// There should be only one nanny per process.
type Nanny struct {
	Name string // Nanny's name.
	// Function that will be called when notifier.Notify returns error.
	// If not specified, uses defaultErrorFunc.
	ErrorFunc ErrorFunc
	timersMu  sync.RWMutex
	timers    map[string]*Timer // Map of program names (Signal.Name) to their timers.
}

// Signal represents program calling nanny to notify with given notifier if
// this program does not call again within NextSignal + MaxDeviation.
type Signal struct {
	// Name of program being monitored.
	// Should be unique for each instance of a program.
	Name       string
	Notifier   notifier.Notifier // What notifier to use.
	NextSignal time.Duration     // Notify after reaching this timeout.
	AllClear   bool              // Activate optional all-clear notification
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
	return n.handleWithPersistence(s, nil)
}

// HandleWithPersistence handles a signal and runs persist before its timer is
// armed. Calls for the same signal are serialized with expiry callbacks.
func (n *Nanny) HandleWithPersistence(s Signal, persist func(Signal, time.Time)) error {
	return n.handleWithPersistence(s, persist)
}

func (n *Nanny) handleWithPersistence(s Signal, persist func(Signal, time.Time)) error {
	vs, err := n.validate(s)
	if err != nil {
		return fmt.Errorf("signal is invalid: %w", err)
	}

	n.handle(vs, persist)
	return nil
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
func (n *Nanny) handle(s validSignal, persist func(Signal, time.Time)) {
	n.timersMu.Lock()
	if n.timers == nil {
		n.timers = make(map[string]*Timer)
	}
	timer := n.timers[s.Name]
	if timer == nil {
		timer = &Timer{signal: s, nanny: n, generation: 1}
		timer.lock.Lock()
		n.timers[s.Name] = timer
		n.timersMu.Unlock()
		timer.initializeLocked(persist)
		timer.lock.Unlock()
		return
	}
	n.timersMu.Unlock()
	timer.resetAfterHeartbeat(s, persist)
}

// GetTimer returns time.Timer when given program name is already registered or
// nil.
func (n *Nanny) GetTimer(name string) *Timer {
	n.timersMu.RLock()
	defer n.timersMu.RUnlock()
	return n.timers[name]
}

// SetTimer sets new timer for given program name.
func (n *Nanny) SetTimer(name string, timer *Timer) {
	n.timersMu.Lock()
	defer n.timersMu.Unlock()
	if n.timers == nil {
		n.timers = make(map[string]*Timer)
	}
	n.timers[name] = timer
}

// GetTimers returns a slice of currently open timers
func (n *Nanny) GetTimers() []*Timer {
	n.timersMu.RLock()
	defer n.timersMu.RUnlock()
	timers := make([]*Timer, 0, len(n.timers))
	for _, timer := range n.timers {
		timers = append(timers, timer)
	}
	return timers
}
