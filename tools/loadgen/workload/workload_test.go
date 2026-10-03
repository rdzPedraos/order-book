package workload

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func buildRequests(c *require.Assertions, profile string, count int) []Request {
	generator, err := NewGenerator(profile, 20, 1)
	c.NoError(err)

	requests := make([]Request, 0, count)
	for range count {
		requests = append(requests, generator.BuildNext())
	}

	return requests
}

func countMarkets(requests []Request) int {
	markets := 0

	for _, request := range requests {
		if request.Kind == KindCreate && request.Limit == nil {
			markets++
		}
	}

	return markets
}

func TestProfileA(t *testing.T) {
	t.Run("everything crosses: a buy and a sell at one price, by turns", func(t *testing.T) {
		c := require.New(t)

		requests := buildRequests(c, "A", 4)

		c.Equal([]string{"BUY", "SELL", "BUY", "SELL"}, []string{requests[0].Side, requests[1].Side, requests[2].Side, requests[3].Side})
		c.Equal(int64(10000), *requests[0].Limit)
		c.Equal(int64(10000), *requests[1].Limit)
		c.NotEqual(requests[0].UserID, requests[1].UserID, "one person never crosses itself")
	})
}

func TestProfileB(t *testing.T) {
	t.Run("90 percent rests and 10 percent are market orders", func(t *testing.T) {
		c := require.New(t)

		requests := buildRequests(c, "B", 1000)

		c.Equal(100, countMarkets(requests))
	})

	t.Run("limit orders never cross: buys below the mid price and sells above", func(t *testing.T) {
		c := require.New(t)

		for _, request := range buildRequests(c, "B", 1000) {
			if request.Limit == nil {
				continue
			}

			if request.Side == "BUY" {
				c.Less(*request.Limit, int64(10000))
			} else {
				c.Greater(*request.Limit, int64(10000))
			}
		}
	})

	t.Run("a market buy is by amount and a market sell by quantity", func(t *testing.T) {
		c := require.New(t)

		for _, request := range buildRequests(c, "B", 1000) {
			if request.Limit != nil {
				continue
			}

			if request.Side == "BUY" {
				c.NotNil(request.Amount)
				c.Nil(request.Quantity)
			} else {
				c.NotNil(request.Quantity)
				c.Nil(request.Amount)
			}
		}
	})
}

func TestProfileC(t *testing.T) {
	t.Run("every second request cancels an order the generator knows", func(t *testing.T) {
		c := require.New(t)
		generator, err := NewGenerator("C", 20, 1)
		c.NoError(err)

		first := generator.BuildNext()
		c.Equal(KindCreate, first.Kind)
		generator.AddOrder(first.UserID, "order-1")

		second := generator.BuildNext()
		c.Equal(KindCancel, second.Kind)
		c.Equal("order-1", second.OrderID)
		c.Equal(first.UserID, second.UserID, "only the owner can cancel it")
	})

	t.Run("with no order to cancel it creates one", func(t *testing.T) {
		c := require.New(t)

		requests := buildRequests(c, "C", 4)

		c.Equal(KindCreate, requests[1].Kind)
	})
}

func TestProfileD(t *testing.T) {
	t.Run("a deep book: resting orders spread over many prices", func(t *testing.T) {
		c := require.New(t)
		prices := map[int64]bool{}

		for _, request := range buildRequests(c, "D", 2000) {
			c.NotNil(request.Limit)
			prices[*request.Limit] = true
		}

		c.Len(prices, 2000)
	})
}

func TestProfileE(t *testing.T) {
	t.Run("the burst sends the orders of B", func(t *testing.T) {
		c := require.New(t)

		c.Equal(countMarkets(buildRequests(c, "B", 1000)), countMarkets(buildRequests(c, "E", 1000)))
	})
}

func TestNewGenerator(t *testing.T) {
	t.Run("an unknown profile", func(t *testing.T) {
		c := require.New(t)

		_, err := NewGenerator("Z", 20, 1)
		c.ErrorIs(err, ErrUnknownProfile)
	})

	t.Run("the same seed gives the same requests", func(t *testing.T) {
		c := require.New(t)

		c.Equal(buildRequests(c, "B", 100), buildRequests(c, "B", 100))
	})
}
