package main

var (
	M_ITEMS     = []mItem{}
	M_ITEM_DICT = make(map[int]mItem)
)

func InitItemCache() error {
	if len(M_ITEMS) != 0 {
		return nil
	}
	var items []mItem
	err := db.Select(&items, "SELECT * FROM m_item")
	if err != nil {
		return err
	}
	M_ITEMS = items
	for _, v := range items {
		M_ITEM_DICT[v.ItemID] = v
	}
	return nil
}
