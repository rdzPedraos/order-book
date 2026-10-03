// Package workload builds the orders of each load profile of the benchmark
// (design D4 of 06-container-infrastructure), around a mid price of R$ 100.00:
//
//   - A: everything crosses, a buy and a sell at the mid price by turns;
//   - B: 90 % rests, buys below the mid price and sells above, and 10 % are
//     market orders that cross them;
//   - C: half the requests cancel an earlier order of its owner;
//   - D: a deep book, resting orders spread over thousands of prices;
//   - E: the orders of B, sent in a burst (see package pace).
//
// The same seed builds the same requests, so two runs are comparable.
package workload

import (
	"errors"
	"math/rand/v2"
	"strconv"
	"sync"
)

const (
	KindCreate = "create"
	KindCancel = "cancel"
)

// In the quote's minimal units: R$ 100.00.
const midPrice int64 = 10000

// How far from the mid price a resting order of B, C and E goes, in cents,
// and how many prices each side of the deep book of D has.
const (
	restingSpread = 500
	deepLevels    = 5000
)

var ErrUnknownProfile = errors.New("unknown profile: use A, B, C, D or E")

// A Limit of nil is a market order: a market buy has an Amount and any other
// order a Quantity. A cancel only has its UserID and OrderID.
type Request struct {
	Kind     string
	UserID   string
	OrderID  string
	Side     string
	Limit    *int64
	Quantity *int64
	Amount   *int64
}

type placedOrder struct {
	userID  string
	orderID string
}

// BuildNext is called by one goroutine; AddOrder may be called by many.
type Generator struct {
	profile string
	people  int
	random  *rand.Rand
	count   int

	lock sync.Mutex
	open []placedOrder
}

func NewGenerator(profile string, people int, seed uint64) (*Generator, error) {
	switch profile {
	case "A", "B", "C", "D", "E":
		return &Generator{profile: profile, people: people, random: rand.New(rand.NewPCG(seed, seed))}, nil
	default:
		return nil, ErrUnknownProfile
	}
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

// An order that profile C may cancel later, by its owner. Other profiles
// never cancel, so they do not keep it.
func (g *Generator) AddOrder(userID, orderID string) {
	if g.profile != "C" {
		return
	}

	g.lock.Lock()
	defer g.lock.Unlock()

	g.open = append(g.open, placedOrder{userID: userID, orderID: orderID})
}

func (g *Generator) BuildNext() Request {
	index := g.count
	g.count++

	switch g.profile {
	case "A":
		return g.buildCrossing(index)
	case "C":
		return g.buildCancelOrResting(index)
	case "D":
		return g.buildDeep(index)
	default:
		return g.buildMostlyResting(index)
	}
}

func (g *Generator) buildCrossing(index int) Request {
	side := "BUY"
	if index%2 == 1 {
		side = "SELL"
	}

	return newLimit(getPersonID(index%g.people), side, midPrice, 1)
}

func (g *Generator) buildMostlyResting(index int) Request {
	if index%10 == 9 {
		return g.buildMarket(index)
	}

	return g.buildResting()
}

// A buy pays at most the highest resting sell, so it always crosses.
func (g *Generator) buildMarket(index int) Request {
	request := Request{Kind: KindCreate, UserID: g.getRandomPerson(), Side: "BUY"}

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

func (g *Generator) buildCancelOrResting(index int) Request {
	if index%2 == 1 {
		if order, ok := g.takeOpenOrder(); ok {
			return Request{Kind: KindCancel, UserID: order.userID, OrderID: order.orderID}
		}
	}

	return g.buildResting()
}

// The oldest open order goes first.
func (g *Generator) takeOpenOrder() (placedOrder, bool) {
	g.lock.Lock()
	defer g.lock.Unlock()

	if len(g.open) == 0 {
		return placedOrder{}, false
	}

	order := g.open[0]
	g.open = g.open[1:]

	return order, true
}

func (g *Generator) buildDeep(index int) Request {
	distance := 1 + int64((index/2)%deepLevels)

	if index%2 == 0 {
		return newLimit(g.getRandomPerson(), "BUY", midPrice-distance, 1)
	}

	return newLimit(g.getRandomPerson(), "SELL", midPrice+distance, 1)
}

func (g *Generator) getRandomPerson() string {
	return getPersonID(g.random.IntN(g.people))
}

func newLimit(userID, side string, price, quantity int64) Request {
	return Request{Kind: KindCreate, UserID: userID, Side: side, Limit: &price, Quantity: &quantity}
}

func ptr[T any](value T) *T {
	return &value
}
