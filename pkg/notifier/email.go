package notifier

import (
	"fmt"

	"github.com/pkg/errors"
	"gopkg.in/gomail.v2"
)

// Email implements Notifier interface for SMTP.
type Email struct {
	From    string
	To      []string
	Subject string
	Body    string

	Server   string
	Port     int
	User     string
	Password string
}

// Notify notifies user via email. It sends message immediately and does not group
// more messages together. We have no idea of importance of monitored program
// and the user might prefer not to wait.
func (n *Email) Notify(msg Message) error {
	m := gomail.NewMessage()
	m.SetHeader("From", n.From)
	m.SetHeader("To", n.To...)
	m.SetHeader("Subject", fmt.Sprintf(n.Subject, msg.Program))
	m.SetBody("text/html", fmt.Sprintf(n.Body, fmt.Sprintf("%s (Meta: %v)", msg.Format(), msg.Meta)))

	d := gomail.NewDialer(n.Server, n.Port, n.User, n.Password)

	if err := d.DialAndSend(m); err != nil {
		return errors.Wrap(err, "unable to notify via email")
	}
	return nil
}

// MarshalJSON marshals the Email notifier into its name "email"
func (n *Email) String() string {
	return "email"
}
