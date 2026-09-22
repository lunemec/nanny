package notifier

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/xmppo/go-xmpp"
)

type xmppSession struct {
	send  func(xmpp.Chat) (int, error)
	close func() error
}

type xmppNotifier struct {
	To       []string
	Server   string
	Port     int
	User     string
	Password string
	Resource string
	NoTLS    bool

	open func(xmpp.Options) (xmppSession, error)
}

// NewXmpp creates a new xmpp notifier from the supplied configuration.
func NewXmpp(To []string,
	Server string,
	Port int,
	User string,
	Password string,
	Resource string,
	NoTLS bool) (Notifier, error) {

	if len(To) == 0 {
		return nil, errors.New("unable to initialize xmpp: To is empty")
	}
	if Server == "" {
		return nil, errors.New("unable to initialize xmpp: Server is empty")
	}
	if User == "" {
		return nil, errors.New("unable to initialize xmpp: User is empty")
	}

	return &xmppNotifier{
		To:       To,
		Server:   Server,
		Port:     Port,
		User:     User,
		Password: Password,
		Resource: Resource,
		NoTLS:    NoTLS,
		open:     openXMPP,
	}, nil
}

func openXMPP(options xmpp.Options) (xmppSession, error) {
	client, err := options.NewClient()
	if err != nil {
		return xmppSession{}, err
	}
	return xmppSession{send: client.Send, close: client.Close}, nil
}

// Notify implements the Notifier interface for xmpp.
func (x *xmppNotifier) Notify(msg Message) error {
	return x.notify(fmt.Sprintf("%s (Meta: %v)", msg.Format(), msg.Meta))
}

// NotifyAllClear implements the Notifier interface for xmpp.
func (x *xmppNotifier) NotifyAllClear(msg Message) error {
	return x.notify(fmt.Sprintf("%s (Meta: %v)", msg.FormatAllClear(), msg.Meta))
}

func (x *xmppNotifier) notify(text string) (err error) {
	options := xmpp.Options{
		Host:                         net.JoinHostPort(x.Server, strconv.Itoa(x.Port)),
		User:                         x.User,
		Password:                     x.Password,
		DialTimeout:                  10 * time.Second,
		Resource:                     x.Resource,
		NoTLS:                        x.NoTLS,
		InsecureAllowUnencryptedAuth: x.NoTLS,
	}

	open := x.open
	if open == nil {
		open = openXMPP
	}
	client, err := open(options)
	if err != nil {
		return fmt.Errorf("unable to connect to xmpp server: %w", err)
	}
	defer func() {
		err = errors.Join(err, client.close())
	}()

	for _, remoteAddress := range x.To {
		if _, err := client.send(xmpp.Chat{Remote: remoteAddress, Text: text}); err != nil {
			return fmt.Errorf("unable to notify via xmpp: %w", err)
		}
	}

	return nil
}

func (x *xmppNotifier) String() string {
	return "xmpp"
}
