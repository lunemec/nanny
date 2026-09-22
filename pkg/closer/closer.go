package closer

import (
	"fmt"
	"io"
	"log/slog"
)

// Close closes any io.Closer and checks for error, which will be logged.
func Close(closer io.Closer) {
	err := closer.Close()
	if err != nil {
		slog.Error("Unable to close resource!", "err", err, "type", fmt.Sprintf("%T", closer))
	}
}
