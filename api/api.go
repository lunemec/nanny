package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"nanny/pkg/nanny"
	"nanny/pkg/notifier"

	"github.com/gorilla/mux"
	log "github.com/mgutz/logxi"
	"github.com/pkg/errors"
)

// Server is Nanny API Server.
type Server struct {
	Name      string
	Notifiers notifiers

	Server *http.Server

	nanny nanny.Nanny
}

// Signal represents incomming JSON-encoded data to process.
type Signal struct {
	// Name of program being monitored.
	// This must be unique for each monitored instance.
	Name       string            `json:"name"`
	Notifier   string            `json:"notifier"`    // What notifier to use.
	NextSignal uint              `json:"next_signal"` // After how many seconds to expect next call.
	Meta       map[string]string `json:"meta"`        // Metadata for this signal, may contain custom data.
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
type handlerWithDeps func(*nanny.Nanny, notifiers, http.ResponseWriter, *http.Request) error
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

	return router(&a.nanny, a.Notifiers), nil
}

func router(nanny *nanny.Nanny, notifiers notifiers) *mux.Router {
	router := mux.NewRouter()
	// Clarify this is API.
	apiRouter := router.PathPrefix("/api").Subrouter()
	apiRouter.Handle("/version", panicWrap(headerWrap(errWrap(versionHandler))))
	// In case of future API changes, nanny will support older versions of API.
	v1Router := apiRouter.PathPrefix("/v1").Subrouter()
	v1Router.Handle("/signal", panicWrap(headerWrap(errWrap(depWrap(nanny, notifiers, signalHandler))))).Methods("POST")

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
func depWrap(nanny *nanny.Nanny, notifiers notifiers, handler handlerWithDeps) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		return handler(nanny, notifiers, w, r)
	}
}

// versionHandler simply returns version of this nanny.
func versionHandler(w http.ResponseWriter, req *http.Request) error {
	w.WriteHeader(http.StatusOK)
	_, err := io.WriteString(w, "Nanny v0.1")
	return errors.Wrap(err, "unable to reply with version")
}

// signalHandler handles incomming register/ping signal from a source.
func signalHandler(n *nanny.Nanny, notifiers notifiers, w http.ResponseWriter, req *http.Request) error {
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

	s := nanny.Signal{
		Name:       signal.Name,
		Notifier:   notif,
		NextSignal: time.Duration(signal.NextSignal) * time.Second,
		Meta:       signal.Meta,
	}
	return n.Handle(s)
}

// Close closes any io.Closer and checks for error, which will be logged.
func Close(closer io.Closer) {
	err := closer.Close()
	if err != nil {
		log.Error("Unable to close resource!", "err", err, "type", fmt.Sprintf("%T", closer))
	}
}
