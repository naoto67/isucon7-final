package main

import (
	"context"
	"fmt"
	"log"
	"math/big"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
)

// 部屋のロックを取りタイムスタンプを更新する
//
// トランザクション開始後この関数を呼ぶ前にクエリを投げると、
// そのトランザクション中の通常のSELECTクエリが返す結果がロック取得前の
// 状態になることに注意 (keyword: MVCC, repeatable read).
func updateRoomTime(tx *sqlx.Tx, roomName string, reqTime int64) (int64, bool) {
	// See page 13 and 17 in https://www.slideshare.net/ichirin2501/insert-51938787
	var roomTime int64
	err := tx.Get(&roomTime, "SELECT time FROM room_time WHERE room_name = ? FOR UPDATE", roomName)
	if err != nil {
		log.Println(err)
		return 0, false
	}

	var currentTime int64
	err = tx.Get(&currentTime, "SELECT floor(unix_timestamp(current_timestamp(3))*1000)")
	if err != nil {
		log.Println(err)
		return 0, false
	}
	if roomTime > currentTime {
		log.Println("room time is future")
		return 0, false
	}
	if reqTime != 0 {
		if reqTime < currentTime {
			log.Println("reqTime is past")
			return 0, false
		}
	}

	_, err = tx.Exec("UPDATE room_time SET time = ? WHERE room_name = ?", currentTime, roomName)
	if err != nil {
		log.Println(err)
		return 0, false
	}

	return currentTime, true
}

func addIsu(roomName string, reqIsu *big.Int, reqTime int64) bool {
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

	_, err = tx.Exec("INSERT INTO adding(room_name, time, isu) VALUES (?, ?, '0')", roomName, reqTime)
	if err != nil {
		log.Println("INSERT INTO adding: err", err)

		fmt.Println("DEBUG: addIsu: reqIsu", reqIsu)
		fmt.Println("DEBUG: addIsu: reqTime", reqTime)
		_, err = tx.Exec("INSERT INTO adding(room_name, time, isu) VALUES (?, ?, '0') ON DUPLICATE KEY UPDATE isu=isu", roomName, reqTime)
		if err != nil {
			log.Println(err)
			tx.Rollback()
			return false
		}

		var isuStr string
		err = tx.QueryRow("SELECT isu FROM adding WHERE room_name = ? AND time = ? FOR UPDATE", roomName, reqTime).Scan(&isuStr)
		if err != nil {
			log.Println(err)
			tx.Rollback()
			return false
		}
		isu := str2big(isuStr)
		isu.Add(isu, reqIsu)
		_, err = tx.Exec("UPDATE adding SET isu = ? WHERE room_name = ? AND time = ?", isu.String(), roomName, reqTime)
		if err != nil {
			log.Println(err)
			tx.Rollback()
			return false
		}
	}
	if err := tx.Commit(); err != nil {
		log.Println(err)
		return false
	}

	return true
}

func buyItem(roomName string, itemID int, countBought int, reqTime int64) bool {
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

	var countBuying int
	err = tx.Get(&countBuying, "SELECT COUNT(*) FROM buying WHERE room_name = ? AND item_id = ?", roomName, itemID)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}
	if countBuying != countBought {
		tx.Rollback()
		log.Println(roomName, itemID, countBought+1, " is already bought")
		return false
	}

	totalMilliIsu := new(big.Int)
	var addings []Adding
	err = tx.Select(&addings, "SELECT isu FROM adding WHERE room_name = ? AND time <= ?", roomName, reqTime)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}

	for _, a := range addings {
		totalMilliIsu.Add(totalMilliIsu, new(big.Int).Mul(str2big(a.Isu), big.NewInt(1000)))
	}

	var buyings []Buying
	err = tx.Select(&buyings, "SELECT item_id, ordinal, time FROM buying WHERE room_name = ?", roomName)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}
	dict, _ := FetchMItems()
	for _, b := range buyings {
		var item mItem
		item, _ = dict[b.ItemID]
		cost := new(big.Int).Mul(item.GetPrice(b.Ordinal), big.NewInt(1000))
		totalMilliIsu.Sub(totalMilliIsu, cost)
		if b.Time <= reqTime {
			gain := new(big.Int).Mul(item.GetPower(b.Ordinal), big.NewInt(reqTime-b.Time))
			totalMilliIsu.Add(totalMilliIsu, gain)
		}
	}

	var item mItem
	item, _ = dict[itemID]
	need := new(big.Int).Mul(item.GetPrice(countBought+1), big.NewInt(1000))
	if totalMilliIsu.Cmp(need) < 0 {
		log.Println("not enough")
		tx.Rollback()
		return false
	}

	_, err = tx.Exec("INSERT INTO buying(room_name, item_id, ordinal, time) VALUES(?, ?, ?, ?)", roomName, itemID, countBought+1, reqTime)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Println(err)
		return false
	}

	return true
}

func getStatus(roomName string) (*GameStatus, error) {
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}

	currentTime, ok := updateRoomTime(tx, roomName, 0)
	if !ok {
		tx.Rollback()
		return nil, fmt.Errorf("updateRoomTime failure")
	}

	mItems, _ := FetchMItems()
	addings := []Adding{}
	err = tx.Select(&addings, "SELECT time, isu FROM adding WHERE room_name = ?", roomName)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	buyings := []Buying{}
	err = tx.Select(&buyings, "SELECT item_id, ordinal, time FROM buying WHERE room_name = ?", roomName)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	status, err := calcStatus(currentTime, mItems, addings, buyings)
	if err != nil {
		return nil, err
	}

	// calcStatusに時間がかかる可能性があるので タイムスタンプを取得し直す
	latestTime, err := getCurrentTime()
	if err != nil {
		return nil, err
	}

	status.Time = latestTime
	return status, err
}

func calcStatus(currentTime int64, mItems map[int]mItem, addings []Adding, buyings []Buying) (*GameStatus, error) {
	var (
		// 1ミリ秒に生産できる椅子の単位をミリ椅子とする
		totalMilliIsu = big.NewInt(0)
		totalPower    = big.NewInt(0)

		itemPower    = map[int]*big.Int{}    // ItemID => Power
		itemPrice    = map[int]*big.Int{}    // ItemID => Price
		itemOnSale   = map[int]int64{}       // ItemID => OnSale
		itemBuilt    = map[int]int{}         // ItemID => BuiltCount
		itemBought   = map[int]int{}         // ItemID => CountBought
		itemBuilding = map[int][]Building{}  // ItemID => Buildings
		itemPower0   = map[int]Exponential{} // ItemID => currentTime における Power
		itemBuilt0   = map[int]int{}         // ItemID => currentTime における BuiltCount

		addingAt = map[int64]Adding{}   // Time => currentTime より先の Adding
		buyingAt = map[int64][]Buying{} // Time => currentTime より先の Buying
	)

	for itemID := range mItems {
		itemPower[itemID] = big.NewInt(0)
		itemBuilding[itemID] = []Building{}
	}

	for _, a := range addings {
		// adding は adding.time に isu を増加させる
		if a.Time <= currentTime {
			totalMilliIsu.Add(totalMilliIsu, new(big.Int).Mul(str2big(a.Isu), big.NewInt(1000)))
		} else {
			addingAt[a.Time] = a
		}
	}

	for _, b := range buyings {
		// buying は 即座に isu を消費し buying.time からアイテムの効果を発揮する
		itemBought[b.ItemID]++
		m := mItems[b.ItemID]
		totalMilliIsu.Sub(totalMilliIsu, new(big.Int).Mul(m.GetPrice(b.Ordinal), big.NewInt(1000)))

		if b.Time <= currentTime {
			itemBuilt[b.ItemID]++
			power := m.GetPower(itemBought[b.ItemID])
			totalMilliIsu.Add(totalMilliIsu, new(big.Int).Mul(power, big.NewInt(currentTime-b.Time)))
			totalPower.Add(totalPower, power)
			itemPower[b.ItemID].Add(itemPower[b.ItemID], power)
		} else {
			buyingAt[b.Time] = append(buyingAt[b.Time], b)
		}
	}

	for _, m := range mItems {
		itemPower0[m.ItemID] = big2exp(itemPower[m.ItemID])
		itemBuilt0[m.ItemID] = itemBuilt[m.ItemID]
		price := m.GetPrice(itemBought[m.ItemID] + 1)
		itemPrice[m.ItemID] = price
		if 0 <= totalMilliIsu.Cmp(new(big.Int).Mul(price, big.NewInt(1000))) {
			itemOnSale[m.ItemID] = 0 // 0 は 時刻 currentTime で購入可能であることを表す
		}
	}

	schedule := []Schedule{
		Schedule{
			Time:       currentTime,
			MilliIsu:   big2exp(totalMilliIsu),
			TotalPower: big2exp(totalPower),
		},
	}

	// currentTime から 1000 ミリ秒先までシミュレーションする
	for t := currentTime + 1; t <= currentTime+1000; t++ {
		totalMilliIsu.Add(totalMilliIsu, totalPower)
		updated := false

		// 時刻 t で発生する adding を計算する
		if a, ok := addingAt[t]; ok {
			updated = true
			totalMilliIsu.Add(totalMilliIsu, new(big.Int).Mul(str2big(a.Isu), big.NewInt(1000)))
		}

		// 時刻 t で発生する buying を計算する
		if _, ok := buyingAt[t]; ok {
			updated = true
			updatedID := map[int]bool{}
			for _, b := range buyingAt[t] {
				m := mItems[b.ItemID]
				updatedID[b.ItemID] = true
				itemBuilt[b.ItemID]++
				power := m.GetPower(b.Ordinal)
				itemPower[b.ItemID].Add(itemPower[b.ItemID], power)
				totalPower.Add(totalPower, power)
			}
			for id := range updatedID {
				itemBuilding[id] = append(itemBuilding[id], Building{
					Time:       t,
					CountBuilt: itemBuilt[id],
					Power:      big2exp(itemPower[id]),
				})
			}
		}

		if updated {
			schedule = append(schedule, Schedule{
				Time:       t,
				MilliIsu:   big2exp(totalMilliIsu),
				TotalPower: big2exp(totalPower),
			})
		}

		// 時刻 t で購入可能になったアイテムを記録する
		for itemID := range mItems {
			if _, ok := itemOnSale[itemID]; ok {
				continue
			}
			if 0 <= totalMilliIsu.Cmp(new(big.Int).Mul(itemPrice[itemID], big.NewInt(1000))) {
				itemOnSale[itemID] = t
			}
		}
	}

	gsAdding := []Adding{}
	for _, a := range addingAt {
		gsAdding = append(gsAdding, a)
	}

	gsItems := []Item{}
	for itemID, _ := range mItems {
		gsItems = append(gsItems, Item{
			ItemID:      itemID,
			CountBought: itemBought[itemID],
			CountBuilt:  itemBuilt0[itemID],
			NextPrice:   big2exp(itemPrice[itemID]),
			Power:       itemPower0[itemID],
			Building:    itemBuilding[itemID],
		})
	}

	gsOnSale := []OnSale{}
	for itemID, t := range itemOnSale {
		gsOnSale = append(gsOnSale, OnSale{
			ItemID: itemID,
			Time:   t,
		})
	}

	return &GameStatus{
		Adding:   gsAdding,
		Schedule: schedule,
		Items:    gsItems,
		OnSale:   gsOnSale,
	}, nil
}

func serveGameConn(conn *websocket.Conn, roomName string) {
	ws := NewWebSocket(conn)
	log.Println(ws.RemoteAddr(), "serveGameConn", roomName)

	defer ws.Close()

	go RoomNameTickerHandler(roomName, ws)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chReq := make(chan GameRequest)

	go func() {
		defer cancel()
		for {
			req := GameRequest{}
			err := ws.ReadJSON(&req)
			if err != nil {
				log.Println(err)
				return
			}

			select {
			case chReq <- req:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case req := <-chReq:
			log.Println(req)

			success := false
			switch req.Action {
			case "addIsu":
				success = addIsu(roomName, str2big(req.Isu), req.Time)
			case "buyItem":
				success = buyItem(roomName, req.ItemID, req.CountBought, req.Time)
			default:
				log.Println("Invalid Action")
				return
			}

			if success {
				// GameResponse を返却する前に 反映済みの GameStatus を返す
				status, err := getStatus(roomName)
				if err != nil {
					log.Println(err)
					return
				}

				err = ws.WriteJson(status)
				if err != nil {
					log.Println(err)
					return
				}
			}

			err := ws.WriteJson(GameResponse{
				RequestID: req.RequestID,
				IsSuccess: success,
			})
			if err != nil {
				log.Println(err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
