package models

import (
	"encoding/json"

	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/money"
)

// The public depth of a book: its best bids, highest first, and its best
// asks, lowest first.
type OrderBook struct {
	Book string
	Bids []Level
	Asks []Level
}

type orderBookJSON struct {
	Book string      `json:"book"`
	Bids []levelJSON `json:"bids"`
	Asks []levelJSON `json:"asks"`
}

type levelJSON struct {
	Price  string `json:"price"`
	Volume string `json:"volume"`
	Orders int    `json:"orders"`
}

// Prices are decimal strings in the book's quote, volumes in its base.
func (o OrderBook) MarshalJSON() ([]byte, error) {
	book, err := books.Normalize(o.Book)
	if err != nil {
		return nil, err
	}

	bids, err := formatLevels(book, o.Bids)
	if err != nil {
		return nil, err
	}

	asks, err := formatLevels(book, o.Asks)
	if err != nil {
		return nil, err
	}

	return json.Marshal(orderBookJSON{Book: book.ID, Bids: bids, Asks: asks})
}

func formatLevels(book books.Book, levels []Level) ([]levelJSON, error) {
	formatted := make([]levelJSON, 0, len(levels))

	for _, level := range levels {
		price, err := money.Format(book.Quote, level.Price)
		if err != nil {
			return nil, err
		}

		volume, err := money.Format(book.Base, level.Volume)
		if err != nil {
			return nil, err
		}

		formatted = append(formatted, levelJSON{Price: price, Volume: volume, Orders: level.Orders})
	}

	return formatted, nil
}
