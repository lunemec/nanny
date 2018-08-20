package notifier

import (
	"fmt"
	"time"
)

// Notifier interface is used by Nanny to notify user on different outputs/services.
type Notifier interface {
	Notify(Message) error
	String() string
}

// Message is used with Notifier's Notify to customise messages sent via different
// channels.
type Message struct {
	Nanny      string        // Nanny's name
	Program    string        // Program's name
	NextSignal time.Duration // How long have we not heard from program.
	Meta       map[string]string
}

// Format is deafult Message formatter, used to serialize information for some notifiers.
// This is intended for future use, mainly the ability to set message format from config
// or from API.
func (m *Message) Format() string {
	return fmt.Sprintf("%s: I did not hear from \"%s\" in %s!", m.Nanny, m.Program, m.NextSignal)
}
