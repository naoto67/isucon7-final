package main

import (
	"sync"

	"github.com/gorilla/websocket"
)

var autoIncrementInt AutoIncrementInt

type AutoIncrementInt struct {
	count int
	mux   sync.Mutex
}

func (a AutoIncrementInt) Fetch() int {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.count++
	return a.count
}

type WebSocket struct {
	mux sync.Mutex
	ID  int

	*websocket.Conn
}

func NewWebSocket(conn *websocket.Conn) *WebSocket {
	id := autoIncrementInt.Fetch()
	return &WebSocket{
		ID:   id,
		Conn: conn,
	}
}

func (ws *WebSocket) WriteJson(v interface{}) error {
	ws.mux.Lock()
	defer ws.mux.Lock()
	return ws.WriteJSON(v)
}
