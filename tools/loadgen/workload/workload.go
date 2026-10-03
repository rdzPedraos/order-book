// Package workload builds the orders loadgen sends, around a mid price of
// R$ 100.00: 90 % are limit orders that rest, buys below the mid price and
// sells above, and 10 % are market orders that cross them. Profiles B and E
// of design D4 of 06-container-infrastructure send these same orders and
// differ only in their rate (see package pace).
//
// The same seed builds the same requests, so two runs are comparable.
package workload

import (
	"math/rand/v2"
	"strconv"
)

// In the quote's minimal units: R$ 100.00.
const midPrice int64 = 10000

// How far from the mid price a resting order goes, in cents.
const restingSpread = 500

// A Limit of nil is a market order: a market buy has an Amount and any other
// order a Quantity.
type Request struct {
	UserID   string
	Side     string
	Limit    *int64
	Quantity *int64
	Amount   *int64
}

// BuildNext is called by one goroutine.
type Generator struct {
	people int
	random *rand.Rand
	count  int
}

func NewGenerator(people int, seed uint64) *Generator {
	return &Generator{people: people, random: rand.New(rand.NewPCG(seed, seed))}
}

func ListPeople(count int) []string {
	people := make([]string, 0, count)
	for index := range count {
		people = append(people, getPersonID(index))
	}

	return people
}

func getPersonID(index int) string {
	return "loadgen-" + strconv.Itoa(index)
}

func (g *Generator) BuildNext() Request {
	index := g.count
	g.count++

	if index%10 == 9 {
		return g.buildMarket(index)
	}

	return g.buildResting()
}

// A buy pays at most the highest resting sell, so it always crosses.
func (g *Generator) buildMarket(index int) Request {
	request := Request{UserID: g.getRandomPerson(), Side: "BUY"}

	if (index/10)%2 == 0 {
		request.Amount = ptr(midPrice + restingSpread)
	} else {
		request.Side, request.Quantity = "SELL", ptr(int64(1))
	}

	return request
}

func (g *Generator) buildResting() Request {
	distance := 1 + g.random.Int64N(restingSpread)
	quantity := 1 + g.random.Int64N(5)

	if g.random.IntN(2) == 0 {
		return newLimit(g.getRandomPerson(), "BUY", midPrice-distance, quantity)
	}

	return newLimit(g.getRandomPerson(), "SELL", midPrice+distance, quantity)
}

func (g *Generator) getRandomPerson() string {
	return getPersonID(g.random.IntN(g.people))
}

func newLimit(userID, side string, price, quantity int64) Request {
	return Request{UserID: userID, Side: side, Limit: &price, Quantity: &quantity}
}

func ptr[T any](value T) *T {
	return &value
}
