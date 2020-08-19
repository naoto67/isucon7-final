package main

import (
	"fmt"
	"log"
	"time"
)

var (
	// map[roomName][]websocketConnection
	wsConnsMap = make(map[string][]*WebSocket)
)

func RoomNameTickerHandler(roomName string, ws *WebSocket) {
	if conns, ok := wsConnsMap[roomName]; ok {
		wsConnsMap[roomName] = append(wsConnsMap[roomName], ws)
		fmt.Println("RoomNameTickerHandler: len(conns)", len(conns))
		fmt.Println("RoomNameTickerHandler: ok", ok)

		return
	}
	wsConnsMap[roomName] = append(wsConnsMap[roomName], ws)

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
				return
			}
		}
	}
}
