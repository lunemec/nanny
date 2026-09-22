package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"nanny/pkg/closer"
	"nanny/pkg/nanny"
	"nanny/pkg/notifier"
	"nanny/pkg/storage"
	"nanny/pkg/version"

	"github.com/gorilla/mux"
)

// Server is Nanny API Server.
type Server struct {
	Name      string          // Name of this Nanny.
	Notifiers notifiers       // Enabled notifiers.
	Storage   storage.Storage // What to use as persistence system.

	nanny nanny.Nanny
}

// Signal represents incomming JSON-encoded data to process.
type Signal struct {
	// Name of program being monitored.
	// IP address of caller is appended to the name so it may be non-unique.
	Name     string `json:"name"`
	Notifier string `json:"notifier"` // What notifier to use.
	// After how many seconds to expect next call.
	// May contain "10s", "1h": https://golang.org/pkg/time/#ParseDuration
	NextSignal string `json:"next_signal"`
	// Activate optional all-clear notification that is sent when a call is received after an alert was sent
	AllClear bool              `json:"all_clear"`
	Meta     map[string]string `json:"meta"` // Metadata for this signal, may contain custom data.
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

func (e *httpError) Unwrap() error {
	return e.Err
}

type handler func(http.ResponseWriter, *http.Request) error
type handlerWithDeps func(*nanny.Nanny, notifiers, storage.Storage, http.ResponseWriter, *http.Request) error
type notifiers map[string]notifier.Notifier

// routes contain map of URLs -> Names of routes. This hashmap is created and written to
// only once *before* the server starts. DO NOT WRITE TO IT!
var routes = map[string]string{}

// Handler returns http.Handler and error. This way we can customise
// http.Server and just pass in our handlers.
func (a *Server) Handler() (http.Handler, error) {
	if len(a.Notifiers) == 0 {
		return nil, errors.New("no notifier is set, enable at least one in config")
	}
	if a.Name != "" {
		a.nanny.Name = a.Name
	}

	a.nanny.ErrorFunc = func(err error) {
		slog.Error("Notify error", "err", err)
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
		slog.Warn(msg)
		return
	}
	// create callback func using our storage.
	callbackFunc := makeCallbackFunc(store)

	// Create nanny timers from persisted signals.
	for _, signal := range signals {
		// If NextSignal would be in the past, notify user, and delete it.
		if signal.NextSignal.Before(time.Now()) {
			msg := "Found previously stored notifier that is stale. Please check " +
				"this program manually."
			slog.Warn(msg, "program", signal.Name, "should_notify", signal.NextSignal.String())
			err = store.Remove(signal)
			if err != nil {
				slog.Error("Unable to remove stale signal.", "err", err)
			}
			continue
		}

		notif, ok := notifiers[signal.Notifier]
		if !ok {
			msg := "Unable to find previously stored notifier. It may have been " +
				"disabled. Please check this program manually."
			slog.Warn(msg, "program", signal.Name)
		}
		s := nanny.Signal{
			Name:       signal.Name,
			Notifier:   notif,
			NextSignal: time.Until(signal.NextSignal),
			AllClear:   signal.AllClear,
			Meta:       signal.Meta,

			CallbackFunc: callbackFunc,
		}

		err = n.Handle(s)
		if err != nil {
			msg := "Unable to create signal handler from previous run," +
				" please check this program manually."
			slog.Warn(msg, "program", signal.Name, "err", err)
			continue
		}
		slog.Info("Loaded persisted signal successful.",
			"program", signal.Name,
			"next_signal", s.NextSignal.String(),
			"all_clear", s.AllClear,
			"meta", s.Meta,
			"notifier", signal.Notifier)
	}
}

// makeCallbackFunc creates new function that can be used as nanny.Signal callback
// while injecting storage dependency. This is used to remove signal from persistent
// storage.
func makeCallbackFunc(store storage.Storage) func(*nanny.Signal) {
	return func(signal *nanny.Signal) {
		err := store.Remove(storage.Signal{Name: signal.Name})
		if err != nil {
			slog.Error("Error removing signal from storage.", "err", err, "signal", signal)
		}
	}
}

func router(n *nanny.Nanny, enabledNotifiers notifiers, store storage.Storage) *mux.Router {
	router := mux.NewRouter()
	// ponytail: one lock keeps timer updates and persistence ordered; use keyed
	// locks only if signal write throughput becomes a measured bottleneck.
	var signalMu sync.Mutex
	serializedSignalHandler := func(n *nanny.Nanny, notifiers notifiers, store storage.Storage, w http.ResponseWriter, req *http.Request) error {
		signalMu.Lock()
		defer signalMu.Unlock()
		return signalHandler(n, notifiers, store, w, req)
	}
	// Clarify this is API.
	apiRouter := router.PathPrefix("/api").Subrouter()
	apiRouter.Handle("/", panicWrap(headerWrap(errWrap(listEndpoints)))).Name("List all available API endpoints.").Methods("GET")
	apiRouter.Handle("/version", panicWrap(headerWrap(errWrap(versionHandler)))).Name("Nanny version.").Methods("GET")
	// In case of future API changes, nanny will support older versions of API.
	v1Router := apiRouter.PathPrefix("/v1").Subrouter()
	v1Router.Handle("/signals", panicWrap(headerWrap(errWrap(depWrap(n, enabledNotifiers, store, getSignalsHandler))))).Name("Show all registered signals.").Methods("GET")
	v1Router.Handle("/signal", panicWrap(headerWrap(errWrap(depWrap(n, enabledNotifiers, store, serializedSignalHandler))))).Name("Register new signal.").Methods("POST")

	err := router.Walk(saveRoutes)
	if err != nil {
		slog.Error("router.Walk doesnt want to walk", "err", err)
	}

	return router
}

// listEndpoints is a handler that reads routes variable. It contains all the URL paths
// available to Nanny.
func listEndpoints(w http.ResponseWriter, req *http.Request) error {
	js, err := json.Marshal(routes)
	if err != nil {
		return fmt.Errorf("unable to marshal url routes to json: %w", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(js)
	if err != nil {
		return fmt.Errorf("unable to write output: %w", err)
	}
	return nil
}

// versionHandler simply returns version of this nanny.
func versionHandler(w http.ResponseWriter, req *http.Request) error {
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, version.VersionString); err != nil {
		return fmt.Errorf("unable to reply with version: %w", err)
	}
	return nil
}

// signalHandler handles incomming register/ping signal from a source.
func signalHandler(n *nanny.Nanny, notifiers notifiers, store storage.Storage, w http.ResponseWriter, req *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	var signal Signal

	dec := json.NewDecoder(req.Body)
	defer closer.Close(req.Body)

	err := dec.Decode(&signal)
	if err != nil {
		return &httpError{
			StatusCode: http.StatusBadRequest,
			Err:        fmt.Errorf("unable to decode JSON: %w", err),
		}
	}

	notif, ok := notifiers[signal.Notifier]
	if !ok {
		return &httpError{
			StatusCode: http.StatusBadRequest,
			Err:        fmt.Errorf("unable to find notifier: %s", signal.Notifier),
		}
	}

	s := constructSignal(signal, notif, store, req)
	err = n.Handle(s)
	if err != nil {
		return fmt.Errorf("unable to handle signal: %w", err)
	}

	err = store.Save(storage.Signal{
		Name:       s.Name,
		Notifier:   signal.Notifier,
		NextSignal: time.Now().Add(s.NextSignal),
		AllClear:   s.AllClear,
		Meta:       s.Meta,
	})

	// This error should not be on the API but only logged. Notifications will still
	// work.
	if err != nil {
		slog.Error("Error saving signal to persistent storage", "err", err)
	}
	// When everything is OK, we should return JSON with "status_code": 200, and
	// message "status": "OK".
	//nolint:errcheck // the response is committed and a write failure is not recoverable here
	w.Write([]byte(`{"status_code":200, "status":"OK"}`))
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
	s := nanny.Signal{
		Name:       constructName(jsonSignal.Name, req),
		Notifier:   notif,
		NextSignal: constructDuration(jsonSignal.NextSignal),
		AllClear:   jsonSignal.AllClear,
		Meta:       jsonSignal.Meta,

		CallbackFunc: func(s *nanny.Signal) {
			err := store.Remove(storage.Signal{Name: s.Name})
			if err != nil {
				slog.Error("Error removing signal from storage.", "err", err, "signal", jsonSignal)
			}
		},
	}
	return s
}

func constructName(name string, req *http.Request) string {
	dontModifyName := req.Header.Get("X-Dont-Modify-Name")
	if dontModifyName != "" {
		return name
	}

	remoteAddr := req.Header.Get("X-Forwarded-For")
	if remoteAddr == "" {
		remoteAddr = req.RemoteAddr
	}

	// Split addr:port and add address to the name in format {programName}@{addr}.
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		slog.Warn("Unable to split host from port, using whole remote address.", "addr", remoteAddr, "err", err)
		return fmt.Sprintf("%s@%s", name, remoteAddr)
	}

	return fmt.Sprintf("%s@%s", name, host)
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

func saveRoutes(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
	path, err := route.GetPathTemplate()
	if err != nil {
		return fmt.Errorf("unable to save route: %w", err)
	}
	routes[path] = route.GetName()
	return nil
}
