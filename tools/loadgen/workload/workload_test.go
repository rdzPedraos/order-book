package workload

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func buildRequests(count int) []Request {
	generator := NewGenerator(20, 1)

	requests := make([]Request, 0, count)
	for range count {
		requests = append(requests, generator.BuildNext())
	}

	return requests
}

func countMarkets(requests []Request) int {
	markets := 0

	for _, request := range requests {
		if request.Limit == nil {
			markets++
		}
	}

	return markets
}

func TestBuildNext(t *testing.T) {
	t.Run("90 percent rests and 10 percent are market orders", func(t *testing.T) {
		c := require.New(t)

		requests := buildRequests(1000)

		c.Equal(100, countMarkets(requests))
	})

	t.Run("limit orders never cross: buys below the mid price and sells above", func(t *testing.T) {
		c := require.New(t)

		for _, request := range buildRequests(1000) {
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

		for _, request := range buildRequests(1000) {
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

func TestNewGenerator(t *testing.T) {
	t.Run("the same seed gives the same requests", func(t *testing.T) {
		c := require.New(t)

		c.Equal(buildRequests(100), buildRequests(100))
	})
}

func TestListPeople(t *testing.T) {
	t.Run("people are numbered from zero", func(t *testing.T) {
		c := require.New(t)

		c.Equal([]string{"loadgen-0", "loadgen-1", "loadgen-2"}, ListPeople(3))
	})
}
