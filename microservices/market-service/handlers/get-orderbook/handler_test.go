package getorderbook

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
	"github.com/rdzpedraos/order-book/microservices/market-service/store/marketdb"
	"github.com/rdzpedraos/order-book/shared/fault"
)

func newBookContext(book, query string) *gofr.Context {
	req := httptest.NewRequest(http.MethodGet, "/market/orderbook/"+book+query, http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"book": book})

	return &gofr.Context{Context: req.Context(), Request: gofrHTTP.NewRequest(req)}
}

func level(side string, price, volume int64, orders int) models.Level {
	return models.Level{Book: "BRL-VIB", Side: side, Price: price, Volume: volume, Orders: orders}
}

func TestHandle(t *testing.T) {
	t.Run("book with several levels", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)
		mock.Levels = []models.Level{level("BUY", 9900, 15, 1), level("BUY", 10000, 30, 2), level("SELL", 10100, 5, 1)}

		data, err := Handle(newBookContext("BRL-VIB", ""))
		c.NoError(err)
		c.Equal(models.OrderBook{
			Book: "BRL-VIB",
			Bids: []models.Level{level("BUY", 10000, 30, 2), level("BUY", 9900, 15, 1)},
			Asks: []models.Level{level("SELL", 10100, 5, 1)},
		}, data)
	})

	t.Run("book in lowercase", func(t *testing.T) {
		c := require.New(t)
		marketdb.InitMock(t)

		data, err := Handle(newBookContext("brl-vib", ""))
		c.NoError(err)
		c.Equal("BRL-VIB", data.(models.OrderBook).Book)
	})

	t.Run("unknown book", func(t *testing.T) {
		c := require.New(t)
		marketdb.InitMock(t)

		data, err := Handle(newBookContext("BTC-USD", ""))
		c.Nil(data)
		c.ErrorIs(err, models.ErrBookNotFound)
	})

	t.Run("explicit depth", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)
		mock.Levels = []models.Level{level("BUY", 9900, 15, 1), level("BUY", 10000, 30, 2), level("BUY", 9800, 1, 1)}

		data, err := Handle(newBookContext("BRL-VIB", "?depth=2"))
		c.NoError(err)
		c.Len(data.(models.OrderBook).Bids, 2)
	})

	t.Run("invalid depth", func(t *testing.T) {
		c := require.New(t)
		marketdb.InitMock(t)

		_, err := Handle(newBookContext("BRL-VIB", "?depth=500"))
		c.ErrorIs(err, models.ErrInvalidDepth)

		_, err = Handle(newBookContext("BRL-VIB", "?depth=zero"))
		c.ErrorIs(err, models.ErrInvalidDepth)
	})

	t.Run("an unavailable database", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		_, err := Handle(newBookContext("BRL-VIB", ""))
		c.ErrorIs(err, fault.ErrServiceUnavailable)
	})
}
