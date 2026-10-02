package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrderBookMarshalJSON(t *testing.T) {
	t.Run("prices in the quote and volumes in the base, as decimal strings", func(t *testing.T) {
		c := require.New(t)

		body, err := json.Marshal(OrderBook{
			Book: "BRL-VIB",
			Bids: []Level{{Price: 10000, Volume: 30, Orders: 2}, {Price: 9900, Volume: 15, Orders: 1}},
			Asks: []Level{{Price: 10100, Volume: 5, Orders: 1}},
		})
		c.NoError(err)
		c.JSONEq(`{
			"book": "BRL-VIB",
			"bids": [{"price": "100.00", "volume": "30", "orders": 2}, {"price": "99.00", "volume": "15", "orders": 1}],
			"asks": [{"price": "101.00", "volume": "5", "orders": 1}]
		}`, string(body))
	})

	t.Run("an empty side is an empty list", func(t *testing.T) {
		c := require.New(t)

		body, err := json.Marshal(OrderBook{Book: "BRL-VIB"})
		c.NoError(err)
		c.JSONEq(`{"book": "BRL-VIB", "bids": [], "asks": []}`, string(body))
	})

	t.Run("an unknown book", func(t *testing.T) {
		c := require.New(t)

		_, err := json.Marshal(OrderBook{Book: "BTC-USD"})
		c.Error(err)
	})
}
