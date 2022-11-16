package packet

import (
	"io"

	"github.com/edelbrocken/engine.io/types"
)

type Type string

// Packet types.
const (
	OPEN    Type = "open"
	CLOSE   Type = "close"
	PING    Type = "ping"
	PONG    Type = "pong"
	MESSAGE Type = "message"
	UPGRADE Type = "upgrade"
	NOOP    Type = "noop"
	ERROR   Type = "error"
)

type Options struct {
	Compress bool `json:"compress"`
}

type Packet struct {
	Type         Type                  `json:"type"`
	Data         io.Reader             `json:"data,omitempty"`
	Options      *Options              `json:"options,omitempty"`
	WsPreEncoded types.BufferInterface `json:"wsPreEncoded,omitempty"`
}
