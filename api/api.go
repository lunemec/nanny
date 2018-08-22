package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"nanny/pkg/nanny"
	"nanny/pkg/notifier"
	"nanny/pkg/storage"
	"nanny/pkg/version"

	"github.com/gorilla/mux"
	log "github.com/mgutz/logxi"
	"github.com/pkg/errors"
)

// Server is Nanny API Server.
type Server struct {
	Name      string          // Name of this Nanny.
	Notifiers notifiers       // Enabled notifiers.
	Storage   storage.Storage // What to use as persistence system.

	Server *http.Server

	nanny nanny.Nanny
}

// Signal represents incomming JSON-encoded data to process.
type Signal struct {
	// Name of program being monitored.
	// IP addres of caller is appended to the name so it may be non-unique.
	Name     string `json:"name"`
	Notifier string `json:"notifier"` // What notifier to use.
	// After how many seconds to expect next call.
	// May contain "10s", "1h": https://golang.org/pkg/time/#ParseDuration
	NextSignal string            `json:"next_signal"`
	Meta       map[string]string `json:"meta"` // Metadata for this signal, may contain custom data.
}

// Error represents JSON error to be sent to user.
type Error struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"error"`
}

type httpError struct {
	Err        error
	StatusCode int
}

// Error implements error interface.
func (e *httpError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

type handler func(http.ResponseWriter, *http.Request) error
type handlerWithDeps func(*nanny.Nanny, notifiers, storage.Storage, http.ResponseWriter, *http.Request) error
type notifiers map[string]notifier.Notifier

// Handler returns http.Handler and error. This way we can customise
// http.Server and just pass in our handlers.
func (a *Server) Handler() (http.Handler, error) {
	if a.Notifiers == nil || len(a.Notifiers) == 0 {
		return nil, errors.New("no notifier is set, enable at least one in config")
	}
	if a.Name != "" {
		a.nanny.Name = a.Name
	}

	a.nanny.ErrorFunc = func(err error) {
		log.Error("Notify error", "err", err)
	}

	// Load persisted signals, if any.
	loadStorage(&a.nanny, a.Notifiers, a.Storage)
	return router(&a.nanny, a.Notifiers, a.Storage), nil
}

// loadStorage loads persisted signals. This function does not return error but logs
// information directly (for better error messages).
func loadStorage(n *nanny.Nanny, notifiers notifiers, store storage.Storage) {
	signals, err := store.Load()
	if err != nil {
		msg := "Unable to load persisted signals. " +
			"There may have been saved signals you will not be notified about! " +
			"Please check services using Nanny manually."
		log.Warn(msg)
		return
	}

	// Create nanny timers from persisted signals.
	for _, signal := range signals {
		// If NextSignal would be in the past, notify user, and delete it.
		if signal.NextSignal.Before(time.Now()) {
			msg := "Found previously stored notifier that is stale. Please check " +
				"this program manually."
			log.Warn(msg, "program", signal.Name, "should_notify", signal.NextSignal.String())
			err = store.Remove(signal)
			if err != nil {
				log.Error("Unable to remove stale signal.", "err", err)
			}
			continue
		}

		notif, ok := notifiers[signal.Notifier]
		if !ok {
			msg := "Unable to find previously stored notifier. It may have been " +
				"disabled. Please check this program manually."
			log.Warn(msg, "program", signal.Name)
		}
		s := nanny.Signal{
			Name:       signal.Name,
			Notifier:   notif,
			NextSignal: time.Until(signal.NextSignal),
			Meta:       signal.Meta,

			CallbackFunc: func(s *nanny.Signal) {
				err := store.Remove(storage.Signal{Name: s.Name})
				if err != nil {
					log.Error("Error removing signal from storage.", "err", err, "signal", signal)
				}
			},
		}

		err = n.Handle(s)
		if err != nil {
			msg := "Unable to create signal handler from previous run," +
				" please check this program manually."
			log.Warn(msg, "program", signal.Name, "err", err)
			continue
		}
		log.Info("Loaded persisted signal successfull.",
			"program", signal.Name,
			"next_signal", s.NextSignal.String(),
			"meta", s.Meta,
			"notifier", signal.Notifier)
	}
}

func router(nanny *nanny.Nanny, notifiers notifiers, store storage.Storage) *mux.Router {
	router := mux.NewRouter()
	// Clarify this is API.
	apiRouter := router.PathPrefix("/api").Subrouter()
	apiRouter.Handle("/version", panicWrap(headerWrap(errWrap(versionHandler))))
	// In case of future API changes, nanny will support older versions of API.
	v1Router := apiRouter.PathPrefix("/v1").Subrouter()
	v1Router.Handle("/signals", panicWrap(headerWrap(errWrap(depWrap(nanny, notifiers, store, getSignalsHandler))))).Methods("GET")
	v1Router.Handle("/signal", panicWrap(headerWrap(errWrap(depWrap(nanny, notifiers, store, signalHandler))))).Methods("POST")

	return router
}

// panicWrap recovers from runtime panics, logs and returns message to user.
func panicWrap(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		defer func() {
			r := recover()
			if r != nil {
				switch t := r.(type) {
				case string:
					err = errors.New(t)
				case error:
					err = t
				default:
					err = errors.New("Unknown error")
				}
				log.Error("Panic recovered", "err", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}()
		handler.ServeHTTP(w, r)
	})
}

// headerWrap only sets Nanny as server, may contain more in future.
func headerWrap(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Nanny")
		handler.ServeHTTP(w, r)
	})
}

// errWrap allows us to have http.Handler that can return error which is handled
// here and encoded as JSON error with correct http status code.
func errWrap(handler handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := handler(w, r)
		if err != nil {
			var e Error

			switch v := err.(type) {
			case *httpError:
				w.WriteHeader(v.StatusCode)
				e = Error{
					StatusCode: v.StatusCode,
					Message:    v.Error(),
				}
			default:
				w.WriteHeader(http.StatusInternalServerError)
				e = Error{
					StatusCode: http.StatusInternalServerError,
					Message:    v.Error(),
				}
			}
			w.Header().Set("Content-Type", "application/json")

			out, err := json.Marshal(e)
			if err != nil {
				log.Error("Error returning JSON error", "err", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, err = w.Write(out)
			if err != nil {
				log.Error("Error writing response with JSON error", "err", err)
				return
			}
			return
		}
	})
}

// depWrap adds notifier dependencies to handler.
func depWrap(nanny *nanny.Nanny, notifiers notifiers, storage storage.Storage, handler handlerWithDeps) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		return handler(nanny, notifiers, storage, w, r)
	}
}

// versionHandler simply returns version of this nanny.
func versionHandler(w http.ResponseWriter, req *http.Request) error {
	w.WriteHeader(http.StatusOK)
	_, err := io.WriteString(w, version.VersionString)
	return errors.Wrap(err, "unable to reply with version")
}

// signalHandler handles incomming register/ping signal from a source.
func signalHandler(n *nanny.Nanny, notifiers notifiers, store storage.Storage, w http.ResponseWriter, req *http.Request) error {
	var signal Signal

	dec := json.NewDecoder(req.Body)
	defer Close(req.Body)

	err := dec.Decode(&signal)
	if err != nil {
		return &httpError{
			StatusCode: http.StatusBadRequest,
			Err:        errors.Wrap(err, "unable to decode JSON"),
		}
	}

	notif, ok := notifiers[signal.Notifier]
	if !ok {
		return &httpError{
			StatusCode: http.StatusBadRequest,
			Err:        errors.Errorf("unable to find notifier: %s", signal.Notifier),
		}
	}

	s := constructSignal(signal, notif, store, req)
	err = n.Handle(s)
	if err != nil {
		return errors.Wrap(err, "unable to handle signal")
	}

	err = store.Save(storage.Signal{
		Name:       s.Name,
		Notifier:   signal.Notifier,
		NextSignal: time.Now().Add(s.NextSignal),
		Meta:       s.Meta,
	})

	// This error should not be on the API but only logged. Notifications will still
	// work.
	if err != nil {
		log.Error("Error saving signal to persistent storage", "err", err)
	}
	return nil
}

func getSignalsHandler(n *nanny.Nanny, notifiers notifiers, store storage.Storage, w http.ResponseWriter, req *http.Request) error {
	w.Header().Set("Content-Type", "application/json")

	signals := n.GetTimers()

	err := json.NewEncoder(w).Encode(&struct {
		NannyName string         `json:"nanny_name"`
		Programs  []*nanny.Timer `json:"signals"`
	}{
		NannyName: n.Name,
		Programs:  signals,
	})

	if err != nil {
		return &httpError{
			StatusCode: http.StatusInternalServerError,
			Err:        err,
		}
	}
	return nil
}

func constructSignal(jsonSignal Signal, notif notifier.Notifier, store storage.Storage, req *http.Request) nanny.Signal {
	remoteAddr := req.Header.Get("X-Forwarded-For")
	if remoteAddr == "" {
		remoteAddr = req.RemoteAddr
	}
	s := nanny.Signal{
		Name:       constructName(jsonSignal.Name, remoteAddr),
		Notifier:   notif,
		NextSignal: constructDuration(jsonSignal.NextSignal),
		Meta:       jsonSignal.Meta,

		CallbackFunc: func(s *nanny.Signal) {
			err := store.Remove(storage.Signal{Name: s.Name})
			if err != nil {
				log.Error("Error removing signal from storage.", "err", err, "signal", jsonSignal)
			}
		},
	}
	return s
}

func constructName(name, remoteAddr string) string {
	var outName string
	// Split addr:port and add address to the name in format {programName}@{addr}.
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		outName = fmt.Sprintf("%s@%s", name, remoteAddr)
		log.Warn("Unable to split host from port, using whole remote address.", "addr", remoteAddr, "err", err)
	} else {
		outName = fmt.Sprintf("%s@%s", name, host)
	}

	return outName
}

func constructDuration(nextSignal string) time.Duration {
	d, err := time.ParseDuration(nextSignal)
	if err != nil {
		seconds, err := strconv.Atoi(nextSignal)
		if err != nil {
			// nextSignal string can't be converted to int, it is nonsense.
			// When we set it to 0, which will cause invalid signal and return
			// error to the user.
			return time.Duration(0)
		}
		d = time.Duration(seconds) * time.Second
	}

	return d
}

// Close closes any io.Closer and checks for error, which will be logged.
func Close(closer io.Closer) {
	err := closer.Close()
	if err != nil {
		log.Error("Unable to close resource!", "err", err, "type", fmt.Sprintf("%T", closer))
	}
}
