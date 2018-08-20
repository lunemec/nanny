package notifier

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
)

// StdErr implements Notifier interface for stderr output.
type StdErr struct{}

// Notify to stderr.
func (n *StdErr) Notify(msg Message) error {
	text := fmt.Sprintf("%s: %s (Meta: %v)\n", time.Now().Format(time.RFC3339), msg.Format(), msg.Meta)
	_, err := os.Stderr.WriteString(text)
	return errors.Wrap(err, "unable to notify via stderr")
}

// MarshalJSON marshals the stderr notifier into a "stderr" string
func (n *StdErr) String() string {
	return "stderr"
}
