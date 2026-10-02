package orderbook

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
)

func limitOrder(side models.Side, price, quantity int64, sequence uint64) *models.Order {
	return &models.Order{
		ID: uuid.Must(uuid.NewV7()), UserID: "user-a", Side: side, Type: models.TypeLimit,
		Price: price, Quantity: quantity, Sequence: sequence,
	}
}

func getLevelPrices(book *Book, side models.Side) []int64 {
	var prices []int64
	for level := range book.Levels(side) {
		prices = append(prices, level.Price)
	}

	return prices
}

func getOrderIDs(level *Level) []uuid.UUID {
	var ids []uuid.UUID
	for order := range level.Orders() {
		ids = append(ids, order.ID)
	}

	return ids
}

func TestPut(t *testing.T) {
	t.Run("order is found by its id", func(t *testing.T) {
		c := require.New(t)
		book := New()
		order := limitOrder(models.SideBuy, 9000, 10, 1)

		book.Put(order)

		found, ok := book.Get(order.ID)
		c.True(ok)
		c.Equal(order, found)
		c.Equal(1, book.Len())
	})

	t.Run("orders of a price keep their arrival order", func(t *testing.T) {
		c := require.New(t)
		book := New()
		first, second := limitOrder(models.SideSell, 9500, 1, 1), limitOrder(models.SideSell, 9500, 2, 2)

		book.Put(first)
		book.Put(second)

		level, ok := book.GetBestLevel(models.SideSell)
		c.True(ok)
		c.Equal([]uuid.UUID{first.ID, second.ID}, getOrderIDs(level))
		c.Equal(int64(3), level.Volume)
	})
}

func TestBestPrice(t *testing.T) {
	t.Run("best bid is the highest buy and best ask the lowest sell", func(t *testing.T) {
		c := require.New(t)
		book := New()
		book.Put(limitOrder(models.SideBuy, 9000, 1, 1))
		book.Put(limitOrder(models.SideBuy, 9100, 1, 2))
		book.Put(limitOrder(models.SideSell, 9600, 1, 3))
		book.Put(limitOrder(models.SideSell, 9500, 1, 4))

		bid, ok := book.GetBestLevel(models.SideBuy)
		c.True(ok)
		c.Equal(int64(9100), bid.Price)

		ask, ok := book.GetBestLevel(models.SideSell)
		c.True(ok)
		c.Equal(int64(9500), ask.Price)
	})

	t.Run("best price after emptying the best level", func(t *testing.T) {
		c := require.New(t)
		book := New()
		best := limitOrder(models.SideBuy, 9100, 1, 1)
		book.Put(best)
		book.Put(limitOrder(models.SideBuy, 9000, 1, 2))

		c.True(book.Remove(best.ID))

		bid, ok := book.GetBestLevel(models.SideBuy)
		c.True(ok)
		c.Equal(int64(9000), bid.Price)
	})

	t.Run("empty side", func(t *testing.T) {
		c := require.New(t)
		book := New()
		order := limitOrder(models.SideSell, 9500, 1, 1)
		book.Put(order)
		book.Remove(order.ID)

		_, ok := book.GetBestLevel(models.SideSell)
		c.False(ok)

		_, ok = book.GetBestLevel(models.SideBuy)
		c.False(ok)
	})

	t.Run("a price that empties and fills again is still the best", func(t *testing.T) {
		c := require.New(t)
		book := New()
		first := limitOrder(models.SideBuy, 9100, 1, 1)
		book.Put(first)
		book.Remove(first.ID)
		book.Put(limitOrder(models.SideBuy, 9100, 2, 2))

		bid, ok := book.GetBestLevel(models.SideBuy)
		c.True(ok)
		c.Equal(int64(2), bid.Volume)
	})
}

func TestRemove(t *testing.T) {
	t.Run("order in the middle of a level", func(t *testing.T) {
		c := require.New(t)
		book := New()
		first, middle, last := limitOrder(models.SideBuy, 9000, 1, 1), limitOrder(models.SideBuy, 9000, 2, 2), limitOrder(models.SideBuy, 9000, 3, 3)
		book.Put(first)
		book.Put(middle)
		book.Put(last)

		c.True(book.Remove(middle.ID))

		level, _ := book.GetBestLevel(models.SideBuy)
		c.Equal([]uuid.UUID{first.ID, last.ID}, getOrderIDs(level))
		c.Equal(int64(4), level.Volume)
		_, found := book.Get(middle.ID)
		c.False(found)
	})

	t.Run("first and last of a level", func(t *testing.T) {
		c := require.New(t)
		book := New()
		first, middle, last := limitOrder(models.SideSell, 9500, 1, 1), limitOrder(models.SideSell, 9500, 2, 2), limitOrder(models.SideSell, 9500, 3, 3)
		book.Put(first)
		book.Put(middle)
		book.Put(last)

		c.True(book.Remove(first.ID))
		c.True(book.Remove(last.ID))

		level, _ := book.GetBestLevel(models.SideSell)
		c.Equal([]uuid.UUID{middle.ID}, getOrderIDs(level))
	})

	t.Run("unknown order", func(t *testing.T) {
		c := require.New(t)

		c.False(New().Remove(uuid.New()))
	})
}

func TestLevels(t *testing.T) {
	t.Run("levels from the best price to the worst", func(t *testing.T) {
		c := require.New(t)
		book := New()
		for i, price := range []int64{9000, 9200, 9100} {
			book.Put(limitOrder(models.SideBuy, price, 1, uint64(i)))
			book.Put(limitOrder(models.SideSell, price+1000, 1, uint64(i+10)))
		}

		c.Equal([]int64{9200, 9100, 9000}, getLevelPrices(book, models.SideBuy))
		c.Equal([]int64{10000, 10100, 10200}, getLevelPrices(book, models.SideSell))
	})

	t.Run("iteration can stop early", func(t *testing.T) {
		c := require.New(t)
		book := New()
		book.Put(limitOrder(models.SideBuy, 9000, 1, 1))
		book.Put(limitOrder(models.SideBuy, 9100, 1, 2))
		order := limitOrder(models.SideBuy, 9100, 1, 3)
		book.Put(order)

		var seen []int64
		for level := range book.Levels(models.SideBuy) {
			seen = append(seen, level.Price)

			break
		}

		level, _ := book.GetBestLevel(models.SideBuy)
		for range level.Orders() {
			break
		}

		c.Equal([]int64{9100}, seen)
		c.True(slices.Contains(getOrderIDs(level), order.ID))
	})
}

func TestReduceQuantity(t *testing.T) {
	t.Run("order keeps its place and its level shrinks", func(t *testing.T) {
		c := require.New(t)
		book := New()
		first, second := limitOrder(models.SideBuy, 9000, 10, 1), limitOrder(models.SideBuy, 9000, 3, 2)
		book.Put(first)
		book.Put(second)

		c.True(book.ReduceQuantity(first.ID, 4))

		level, _ := book.GetBestLevel(models.SideBuy)
		c.Equal([]uuid.UUID{first.ID, second.ID}, getOrderIDs(level))
		c.Equal(int64(7), level.Volume)
		c.Equal(int64(4), first.Quantity)
	})

	t.Run("unknown order", func(t *testing.T) {
		c := require.New(t)

		c.False(New().ReduceQuantity(uuid.New(), 1))
	})
}
