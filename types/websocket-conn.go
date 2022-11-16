package types

import (
	"engine.io/events"
	"github.com/gorilla/websocket"
)

type WebSocketConn struct {
	events.EventEmitter
	*websocket.Conn
}
