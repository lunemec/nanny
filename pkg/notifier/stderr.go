package notifier

import (
	"fmt"
	"os"
	"time"
)

// StdErr implements Notifier interface for stderr output.
type StdErr struct{}

// Notify to stderr.
func (n *StdErr) Notify(msg Message) error {
	text := fmt.Sprintf("%s: %s (Meta: %v)\n", time.Now().Format(time.RFC3339), msg.Format(), msg.Meta)
	_, err := os.Stderr.WriteString(text)
	if err != nil {
		return fmt.Errorf("unable to notify via stderr: %w", err)
	}
	return nil
}

// NotifyAllClear to stderr.
func (n *StdErr) NotifyAllClear(msg Message) error {
	text := fmt.Sprintf("%s: %s (Meta: %v)\n", time.Now().Format(time.RFC3339), msg.FormatAllClear(), msg.Meta)
	_, err := os.Stderr.WriteString(text)
	if err != nil {
		return fmt.Errorf("unable to notify via stderr: %w", err)
	}
	return nil
}

// MarshalJSON marshals the stderr notifier into a "stderr" string
func (n *StdErr) String() string {
	return "stderr"
}
