package api

import (
	"encoding/json"
	"net/http"

	"nanny/pkg/nanny"
	"nanny/pkg/storage"
	"nanny/pkg/version"

	log "github.com/mgutz/logxi"
	"github.com/pkg/errors"
)

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
		w.Header().Set("Server", version.VersionString)
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
