package notifier

import (
	"context"
	"fmt"
	"time"

	mail "github.com/wneessen/go-mail"
)

type emailDelivery struct {
	server            string
	port              int
	user              string
	password          string
	implicitTLS       bool
	allowInsecureAuth bool
}

// Email implements Notifier interface for SMTP.
type Email struct {
	From            string
	To              []string
	Subject         string
	SubjectAllClear string
	Body            string

	Server            string
	Port              int
	User              string
	Password          string
	AllowInsecureAuth bool

	send func(context.Context, *mail.Msg, emailDelivery) error
}

// Notify notifies user via email. It sends message immediately and does not group
// more messages together. We have no idea of importance of monitored program
// and the user might prefer not to wait.
func (n *Email) Notify(msg Message) error {
	return n.notify(msg, false)
}

func (n *Email) NotifyAllClear(msg Message) error {
	return n.notify(msg, true)
}

func (n *Email) notify(msg Message, allClear bool) error {
	subject := n.Subject
	text := msg.Format()
	if allClear {
		subject = n.SubjectAllClear
		text = msg.FormatAllClear()
	}

	message := mail.NewMsg()
	if err := message.From(n.From); err != nil {
		return fmt.Errorf("unable to set email sender: %w", err)
	}
	if err := message.To(n.To...); err != nil {
		return fmt.Errorf("unable to set email recipients: %w", err)
	}
	message.Subject(fmt.Sprintf(subject, msg.Program))
	message.SetBodyString(mail.TypeTextHTML, fmt.Sprintf(n.Body, fmt.Sprintf("%s (Meta: %v)", text, msg.Meta)))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	send := n.send
	if send == nil {
		send = sendEmail
	}
	if err := send(ctx, message, emailDelivery{
		server:            n.Server,
		port:              n.Port,
		user:              n.User,
		password:          n.Password,
		implicitTLS:       n.Port == 465,
		allowInsecureAuth: n.AllowInsecureAuth,
	}); err != nil {
		return fmt.Errorf("unable to notify via email: %w", err)
	}
	return nil
}

func sendEmail(ctx context.Context, message *mail.Msg, delivery emailDelivery) error {
	client, err := newMailClient(delivery)
	if err != nil {
		return err
	}
	return client.DialAndSendWithContext(ctx, message)
}

func newMailClient(delivery emailDelivery) (*mail.Client, error) {
	options := []mail.Option{
		mail.WithPort(delivery.port),
		mail.WithUsername(delivery.user),
		mail.WithPassword(delivery.password),
		mail.WithTimeout(10 * time.Second),
		mail.WithOpportunisticSMTPAuth(smtpAuthTypes(delivery.allowInsecureAuth)...),
	}
	if delivery.implicitTLS {
		options = append(options, mail.WithSSL())
	} else {
		options = append(options, mail.WithTLSPolicy(mail.TLSOpportunistic))
	}
	return mail.NewClient(delivery.server, options...)
}

func smtpAuthTypes(allowInsecure bool) []mail.SMTPAuthType {
	authTypes := []mail.SMTPAuthType{
		mail.SMTPAuthSCRAMSHA256,
		mail.SMTPAuthSCRAMSHA1,
		mail.SMTPAuthCramMD5,
	}
	if allowInsecure {
		return append(authTypes, mail.SMTPAuthPlainNoEnc, mail.SMTPAuthLoginNoEnc)
	}
	return append(authTypes, mail.SMTPAuthPlain, mail.SMTPAuthLogin)
}

// MarshalJSON marshals the Email notifier into its name "email"
func (n *Email) String() string {
	return "email"
}
