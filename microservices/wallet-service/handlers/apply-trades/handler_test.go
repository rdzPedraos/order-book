package applytrades

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/container"
	"gofr.dev/pkg/gofr/logging"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/money"
)

func newContext(t *testing.T) *gofr.Context {
	t.Helper()

	mockContainer, _ := container.NewMockContainer(t)
	mockContainer.Logger = logging.NewLogger(logging.FATAL)

	return &gofr.Context{Context: context.Background(), Container: mockContainer}
}

func tradeMessage(c *require.Assertions, buyerID, sellerID string, quantity, price int64) events.Message {
	trade := events.TradeExecuted{
		BuyOrderID: uuid.New(), SellOrderID: uuid.New(), BuyerID: buyerID, SellerID: sellerID,
		Price: price, Quantity: quantity, Amount: quantity * price,
	}

	message, err := events.NewEventMessage(events.RouteTradeExecuted, "BRL-VIB", uuid.New(), 1, time.Now(), trade)
	c.NoError(err)

	return message
}

func getBalance(mock *walletdb.Mock, userID string, currency money.Currency) models.Balance {
	for _, balance := range mock.Balances {
		if balance.UserID == userID && balance.Currency == currency {
			return balance
		}
	}

	return models.Balance{UserID: userID, Currency: currency}
}

func getTotal(mock *walletdb.Mock, currency money.Currency) int64 {
	var total int64

	for _, balance := range mock.Balances {
		if balance.Currency == currency {
			total += balance.Available + balance.Reserved
		}
	}

	return total
}

func deliver(t *testing.T, messages ...events.Message) error {
	t.Helper()

	return Handle(newContext(t), messages)
}

func TestHandle(t *testing.T) {
	t.Run("payment of a trade", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Balances = []models.Balance{
			{UserID: "carla", Currency: money.BRL, Reserved: 19000},
			{UserID: "victor", Currency: money.VIB, Reserved: 2},
		}

		c.NoError(deliver(t, tradeMessage(c, "carla", "victor", 2, 9500)))

		c.Equal(models.Balance{UserID: "carla", Currency: money.BRL}, getBalance(mock, "carla", money.BRL))
		c.Equal(models.Balance{UserID: "carla", Currency: money.VIB, Available: 2}, getBalance(mock, "carla", money.VIB))
		c.Equal(models.Balance{UserID: "victor", Currency: money.VIB}, getBalance(mock, "victor", money.VIB))
		c.Equal(models.Balance{UserID: "victor", Currency: money.BRL, Available: 19000}, getBalance(mock, "victor", money.BRL))
		c.Len(mock.Movements, 4, "each person pays and receives")
	})

	t.Run("duplicated delivery", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Balances = []models.Balance{
			{UserID: "carla", Currency: money.BRL, Reserved: 38000},
			{UserID: "victor", Currency: money.VIB, Reserved: 4},
		}
		trade := tradeMessage(c, "carla", "victor", 2, 9500)

		c.NoError(deliver(t, trade, trade))
		c.NoError(deliver(t, trade))

		c.Equal(models.Balance{UserID: "carla", Currency: money.BRL, Reserved: 19000}, getBalance(mock, "carla", money.BRL))
		c.Len(mock.Movements, 4)
	})

	t.Run("conservation", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Balances = []models.Balance{
			{UserID: "ana", Currency: money.BRL, Reserved: 100000}, {UserID: "ana", Currency: money.VIB, Reserved: 10},
			{UserID: "beto", Currency: money.BRL, Reserved: 100000}, {UserID: "beto", Currency: money.VIB, Reserved: 10},
		}
		brlBefore, vibBefore := getTotal(mock, money.BRL), getTotal(mock, money.VIB)

		c.NoError(deliver(t, tradeMessage(c, "ana", "beto", 3, 9500), tradeMessage(c, "beto", "ana", 2, 9800)))

		c.Equal(brlBefore, getTotal(mock, money.BRL))
		c.Equal(vibBefore, getTotal(mock, money.VIB))
	})

	t.Run("an unavailable database is retried", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(deliver(t, tradeMessage(c, "carla", "victor", 2, 9500)), mock.Err)
	})

	t.Run("a trade that cannot be read is left out of its batch", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Balances = []models.Balance{
			{UserID: "carla", Currency: money.BRL, Reserved: 19000},
			{UserID: "victor", Currency: money.VIB, Reserved: 2},
		}
		unreadable := events.Message{ID: uuid.New(), Route: events.RouteTradeExecuted, Book: "BRL-VIB", Payload: []byte(`[`)}
		unknownBook := tradeMessage(c, "carla", "victor", 2, 9500)
		unknownBook.Book = "BTC-USD"

		c.NoError(deliver(t, unreadable, unknownBook, tradeMessage(c, "carla", "victor", 2, 9500)))

		c.Len(mock.Movements, 4)
	})

	t.Run("a batch without a readable trade writes nothing", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.NoError(deliver(t, events.Message{ID: uuid.New(), Route: events.RouteTradeExecuted, Book: "BRL-VIB", Payload: []byte(`[`)}))
	})
}
