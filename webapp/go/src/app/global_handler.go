package main

import (
	"fmt"
	"log"
	"time"
)

var (
	// map[roomName][]websocketConnection
	wsConnsMap = make(map[string]map[int]*WebSocket)
)

func RoomNameTickerHandler(roomName string, ws *WebSocket) {
	if _, ok := wsConnsMap[roomName]; ok {
		wsConnsMap[roomName][ws.ID] = ws
		return
	}
	wsConnsMap[roomName] = make(map[int]*WebSocket)
	wsConnsMap[roomName][ws.ID] = ws

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
			fmt.Println("WriteJSON: status", status)
			err = conn.WriteJson(status)
			if err != nil {
				log.Println(err)
				delete(conns, conn.ID)
			}
		}
	}
}
