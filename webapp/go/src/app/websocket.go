package main

import (
	"sync"

	"github.com/gorilla/websocket"
)

type WebSocket struct {
	mux sync.Mutex

	*websocket.Conn
}

func (ws *WebSocket) WriteJson(v interface{}) error {
	ws.mux.Lock()
	defer ws.mux.Lock()
	return ws.WriteJSON(v)
}
