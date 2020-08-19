package main

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

var (
	// map[roomName][]websocketConnection
	wsConnsMap = make(map[string][]*websocket.Conn)
)

func RoomNameTickerHandler(roomName string, ws *websocket.Conn) {
	if _, ok := wsConnsMap[roomName]; ok {
		wsConnsMap[roomName] = append(wsConnsMap[roomName], ws)

		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		<-ticker.C
		status, err := getStatus(roomName)
		if err != nil {
			log.Println(err)
			return
		}

		conns, _ := wsConnsMap[roomName]
		for _, conn := range conns {
			err = conn.WriteJSON(status)
			if err != nil {
				log.Println(err)
				return
			}
		}
	}
}
