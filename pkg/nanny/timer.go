package nanny

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"nanny/pkg/notifier"
)

const maxPendingDeliveries = 64

// Timer encapsulates a signal and its timer
type Timer struct {
	signal     validSignal
	timer      *time.Timer
	nanny      *Nanny
	end        time.Time
	generation uint64
	alerted    bool
	persist    func(Signal, time.Time)

	lock       sync.Mutex
	updateMu   sync.Mutex
	lastUpdate chan struct{}
	deliveries []timerDelivery
	delivering bool
	overflowed bool
}

type timerDelivery struct {
	signal   validSignal
	allClear bool
}

// MarshalJSON marshals a nanny.Timer into JSON. Fields name, notifier, next_signal, all_clear and meta are exported
func (nt *Timer) MarshalJSON() ([]byte, error) {
	nt.lock.Lock()
	signal := nt.signal
	end := nt.end
	nt.lock.Unlock()

	return json.Marshal(&struct {
		Name       string            `json:"name"`
		Notifier   string            `json:"notifier"`
		NextSignal string            `json:"next_signal"`
		AllClear   bool              `json:"all_clear"`
		Meta       map[string]string `json:"meta,omitempty"`
	}{
		Name:       signal.Name,
		Notifier:   signal.Notifier.String(),
		NextSignal: end.Format(time.RFC3339),
		AllClear:   signal.AllClear,
		Meta:       signal.Meta,
	})
}

func (nt *Timer) initialize() {
	nt.lock.Lock()
	signal := nt.signal
	deadline := nt.end
	persist := nt.persist
	nt.lock.Unlock()

	if persist != nil {
		persist(Signal(signal), deadline)
	}

	nt.lock.Lock()
	nt.scheduleLocked()
	nt.lock.Unlock()
}

// Reset updates the nannyTimers signal to reset the timer
func (nt *Timer) Reset(vs validSignal) {
	nt.reset(vs, false, false, nil)
}

func (nt *Timer) resetAfterHeartbeat(vs validSignal, persist func(Signal, time.Time)) {
	nt.reset(vs, vs.AllClear, false, persist)
}

func (nt *Timer) reset(vs validSignal, sendAllClear, forceAllClear bool, persist func(Signal, time.Time)) {
	startDelivery := func() bool {
		complete := nt.beginUpdate()
		defer complete()

		nt.lock.Lock()
		previous := nt.signal
		shouldSendAllClear := sendAllClear && (forceAllClear || nt.alerted)
		if nt.timer != nil {
			nt.timer.Stop()
		}
		nt.signal = vs
		nt.end = time.Now().Add(nt.signal.NextSignal)
		nt.generation++
		nt.alerted = false
		nt.persist = persist
		deadline := nt.end
		nt.lock.Unlock()

		if persist != nil {
			persist(Signal(vs), deadline)
		}

		nt.lock.Lock()
		nt.scheduleLocked()
		start := false
		if shouldSendAllClear {
			start = nt.enqueueDeliveryLocked(timerDelivery{signal: previous, allClear: true})
		}
		nt.lock.Unlock()
		return start
	}()

	if startDelivery {
		go nt.drainDeliveries()
	}
}

// ResetAllClear updates the nannyTimers signal to reset the timer
func (nt *Timer) ResetAllClear(vs validSignal) {
	nt.reset(vs, true, true, nil)
}

func (nt *Timer) scheduleLocked() {
	if nt.signal.NextSignal <= 0 {
		nt.alerted = true
		nt.timer = time.AfterFunc(math.MaxInt64, func() {})
		nt.timer.Stop()
		return
	}
	generation := nt.generation
	delay := max(time.Until(nt.end), 0)
	nt.timer = time.AfterFunc(delay, func() {
		nt.onExpire(generation)
	})
}

func (nt *Timer) onExpire(generation uint64) {
	startDelivery := func() bool {
		complete := nt.beginUpdate()
		defer complete()

		nt.lock.Lock()
		if generation != nt.generation || nt.alerted {
			nt.lock.Unlock()
			return false
		}
		nt.alerted = true
		signal := nt.signal
		persist := nt.persist
		nt.lock.Unlock()

		if persist != nil {
			persist(Signal(signal), time.Time{})
		}

		nt.lock.Lock()
		start := nt.enqueueDeliveryLocked(timerDelivery{signal: signal})
		nt.lock.Unlock()
		return start
	}()

	if startDelivery {
		go nt.drainDeliveries()
	}
}

func (nt *Timer) beginUpdate() func() {
	nt.updateMu.Lock()
	previous := nt.lastUpdate
	current := make(chan struct{})
	nt.lastUpdate = current
	nt.updateMu.Unlock()

	if previous != nil {
		<-previous
	}
	return func() { close(current) }
}

func (nt *Timer) enqueueDeliveryLocked(delivery timerDelivery) bool {
	if len(nt.deliveries) >= maxPendingDeliveries {
		// ponytail: a stuck notifier gets a bounded backlog; later transitions
		// replace the tail so the final remote state still matches the timer.
		nt.deliveries[len(nt.deliveries)-1] = delivery
		nt.overflowed = true
		return false
	}
	nt.deliveries = append(nt.deliveries, delivery)
	if nt.delivering {
		return false
	}
	nt.delivering = true
	return true
}

func (nt *Timer) drainDeliveries() {
	for {
		nt.lock.Lock()
		if len(nt.deliveries) == 0 {
			overflowed := nt.overflowed
			nt.overflowed = false
			nt.delivering = false
			nt.lock.Unlock()
			if overflowed {
				nt.reportNotifyError(errors.New("notification delivery queue overflow: older transitions were dropped"))
			}
			return
		}
		delivery := nt.deliveries[0]
		nt.deliveries[0] = timerDelivery{}
		nt.deliveries = nt.deliveries[1:]
		nt.lock.Unlock()

		nt.deliver(delivery)
	}
}

func (nt *Timer) deliver(delivery timerDelivery) {
	var err error
	if delivery.allClear {
		err = nt.notifyAllClear(delivery.signal)
	} else {
		err = nt.notify(delivery.signal)
		if delivery.signal.CallbackFunc != nil {
			signal := Signal(delivery.signal)
			delivery.signal.CallbackFunc(&signal)
		}
	}

	if err != nil {
		nt.reportNotifyError(nt.wrapNotifyError(delivery.signal, err))
	}
}

func (nt *Timer) wrapNotifyError(signal validSignal, err error) error {
	return fmt.Errorf("error calling notifier %T with signal %+v: %w", signal.Notifier, signal, err)
}

func (nt *Timer) reportNotifyError(err error) {
	if nt.nanny.ErrorFunc == nil {
		defaultErrorFunc(err)
		return
	}
	nt.nanny.ErrorFunc(err)
}

func (nt *Timer) notify(signal validSignal) error {
	name := "Nanny"
	if nt.nanny.Name != "" {
		name = nt.nanny.Name
	}

	return signal.Notifier.Notify(notifier.Message{
		Nanny:      name,
		Program:    signal.Name,
		NextSignal: signal.NextSignal,
		Meta:       signal.Meta,
	})
}

func (nt *Timer) notifyAllClear(signal validSignal) error {
	name := "Nanny"
	if nt.nanny.Name != "" {
		name = nt.nanny.Name
	}

	return signal.Notifier.NotifyAllClear(notifier.Message{
		Nanny:      name,
		Program:    signal.Name,
		NextSignal: signal.NextSignal,
		Meta:       signal.Meta,
	})
}
