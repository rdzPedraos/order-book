package marketdb

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
)

func level(side string, price, volume int64, orders int) models.Level {
	return models.Level{Book: "BRL-VIB", Side: side, Price: price, Volume: volume, Orders: orders}
}

func TestMockUpdateLevels(t *testing.T) {
	t.Run("a new level, a change and an emptied level", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)

		c.NoError(UpdateLevels(&gofr.Context{}, []models.Level{level("BUY", 10000, 10, 1), level("BUY", 10000, 30, 2)}))
		c.Equal([]models.Level{level("BUY", 10000, 30, 2)}, mock.Levels)

		c.NoError(UpdateLevels(&gofr.Context{}, []models.Level{level("BUY", 10000, 0, 0)}))
		c.Empty(mock.Levels)
	})

	t.Run("an unavailable database", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(UpdateLevels(&gofr.Context{}, []models.Level{level("BUY", 10000, 10, 1)}), mock.Err)
		_, err := ListLevels(&gofr.Context{}, "BRL-VIB", "BUY", 20)
		c.ErrorIs(err, mock.Err)
	})
}

func TestMockListLevels(t *testing.T) {
	t.Run("the best levels of a side first, up to depth", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Levels = []models.Level{level("BUY", 9900, 15, 1), level("SELL", 10100, 5, 1), level("BUY", 10000, 30, 2), level("BUY", 9800, 1, 1)}

		bids, err := ListLevels(&gofr.Context{}, "BRL-VIB", "BUY", 2)
		c.NoError(err)
		c.Equal([]models.Level{level("BUY", 10000, 30, 2), level("BUY", 9900, 15, 1)}, bids)

		asks, err := ListLevels(&gofr.Context{}, "BRL-VIB", "SELL", 2)
		c.NoError(err)
		c.Equal([]models.Level{level("SELL", 10100, 5, 1)}, asks)
	})
}
