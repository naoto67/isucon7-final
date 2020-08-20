package main

import (
	"log"
	"math/big"
	"strconv"
)

func str2big(s string) *big.Int {
	x := new(big.Int)
	x.SetString(s, 10)
	return x
}

func big2exp(n *big.Int) Exponential {
	s := n.String()

	if len(s) <= 15 {
		return Exponential{n.Int64(), 0}
	}

	t, err := strconv.ParseInt(s[:15], 10, 64)
	if err != nil {
		log.Panic(err)
	}
	return Exponential{t, int64(len(s) - 15)}
}

func getCurrentTime() (int64, error) {
	var currentTime int64
	err := db.Get(&currentTime, "SELECT floor(unix_timestamp(current_timestamp(3))*1000)")
	if err != nil {
		return 0, err
	}
	return currentTime, nil
}

func CreateRoomTime(roomName string) error {
	_, err := db.Exec("INSERT INTO room_time(room_name, time) VALUES (?, 0) ON DUPLICATE KEY UPDATE time = time", roomName)
	return err
}
