package main

import (
	"fmt"
	"log"
	"math/big"
	"sync"
)

var (
	addIsuMap = map[string][]*big.Int{}
	isuMux    sync.Mutex
)

func pushIsuToMap(roomName string, reqIsu *big.Int, reqTime int64) {
	isuMux.Lock()
	defer isuMux.Unlock()
	key := fmt.Sprintf("%s-%d", roomName, reqTime)
	addIsuMap[key] = append(addIsuMap[key], reqIsu)
}

func popIsuMap(roomName string, reqTime int64) []*big.Int {
	isuMux.Lock()
	defer isuMux.Unlock()
	key := fmt.Sprintf("%s-%d", roomName, reqTime)
	res := addIsuMap[key]
	addIsuMap[key] = []*big.Int{}
	return res
}

func AddIsuFromQueue(roomName string, reqTime int64) bool {
	tx, err := db.Beginx()
	if err != nil {
		log.Println(err)
		return false
	}

	_, ok := updateRoomTime(tx, roomName, reqTime)
	if !ok {
		tx.Rollback()
		return false
	}
	isus := popIsuMap(roomName, reqTime)
	var isu *big.Int
	for _, v := range isus {
		isu.Add(isu, v)
	}

	_, err = tx.Exec("INSERT INTO adding(room_name, time, isu) VALUES (?, ?, ?)", roomName, reqTime, isu.String())
	if err != nil {
		var isuStr string
		err = tx.QueryRow("SELECT isu FROM adding WHERE room_name = ? AND time = ? FOR UPDATE", roomName, reqTime).Scan(&isuStr)
		if err != nil {
			log.Println(err)
			for _, v := range isus {
				pushIsuToMap(roomName, v, reqTime)
			}
			tx.Rollback()
			return false
		}
		isuOnDB := str2big(isuStr)
		isu.Add(isu, isuOnDB)
		_, err = tx.Exec("UPDATE adding SET isu = ? WHERE room_name = ? AND time = ?", isu.String(), roomName, reqTime)
		if err != nil {
			log.Println(err)
			tx.Rollback()
			for _, v := range isus {
				pushIsuToMap(roomName, v, reqTime)
			}
			return false
		}
	}
	if err := tx.Commit(); err != nil {
		log.Println(err)
		for _, v := range isus {
			pushIsuToMap(roomName, v, reqTime)
		}
		return false
	}

	return true
}
