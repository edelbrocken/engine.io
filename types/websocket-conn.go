package types

import (
	"github.com/edelbrocken/engine.io/events"
	"github.com/gorilla/websocket"
)

type WebSocketConn struct {
	events.EventEmitter
	*websocket.Conn
}
