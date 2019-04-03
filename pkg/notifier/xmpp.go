package notifier

import (
	"fmt"
	"strconv"

	"github.com/mattn/go-xmpp"
	"github.com/pkg/errors"
)

type xmppNotifier struct {
	To       []string
	Server   string
	Port     int
	User     string
	Password string
	Resource string
	NoTLS    bool
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
		return nil, errors.New("Unable to initialize xmpp: To is empty")
	}
	if Server == "" {
		return nil, errors.New("Unable to initialize xmpp: Server is empty")
	}
	if User == "" {
		return nil, errors.New("Unable to initialize xmpp: User is empty")
	}

	return &xmppNotifier{
		To,
		Server,
		Port,
		User,
		Password,
		Resource,
		NoTLS,
	}, nil
}

// Notify implements the Notifier interface for xmpp.
func (x *xmppNotifier) Notify(msg Message) error {
	options := xmpp.Options{
		Host:     x.Server + ":" + strconv.Itoa(x.Port),
		User:     x.User,
		Password: x.Password,
		Resource: x.Resource,
		NoTLS:    x.NoTLS,
	}

	client, err := options.NewClient()
	if err != nil {
		return errors.Wrap(err, "unable to connect to xmpp server")
	}

	for _, remoteAddress := range x.To {
		_, err = client.Send(xmpp.Chat{
			Remote: remoteAddress,
			Text:   fmt.Sprintf("%s (Meta: %v)", msg.Format(), msg.Meta),
		})
		if err != nil {
			return errors.Wrap(err, "unable to notify via xmpp")
		}
	}

	return nil
}

func (x *xmppNotifier) String() string {
	return "xmpp"
}
