package storage

import (
	"io"
	"time"
)

// Storage interface is used to persist Nanny data in between restarts or outages.
type Storage interface {
	Load() ([]Signal, error)
	Save(Signal) error
	Remove(Signal) error

	io.Closer
}

// Signal represents stored signal information
type Signal struct {
	Name       string            `xorm:"pk" json:"name"`
	Notifier   string            `json:"notifier"`
	NextSignal time.Time         `json:"next_signal"`
	Meta       map[string]string `json:"meta,omitempty"`
}
