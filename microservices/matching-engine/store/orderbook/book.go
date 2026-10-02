// Package orderbook keeps the state of each book in memory: for each side, its
// price levels with their orders in arrival order, the best price at once and
// any order by its id, so adding and removing an order never walks a list;
// and the book's sequence and the messages and orders it already applied.
package orderbook

import (
	"cmp"
	"iter"
	"maps"
	"slices"

	"github.com/google/uuid"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
)

// Besides its orders, a book remembers its sequence and what it already
// applied, so a repeated message or order has no effect.
type Book struct {
	sides       map[models.Side]*side
	nodes       map[uuid.UUID]*node
	sequence    uint64
	applied     map[uuid.UUID]struct{}
	knownOrders map[uuid.UUID]struct{}

	// The last event already in orders.events: the offset of its command and its index.
	publishedOffset int64
	publishedIndex  int
}

type side struct {
	levels map[int64]*Level
	prices *priceHeap
}

func New() *Book {
	return &Book{
		sides: map[models.Side]*side{
			models.SideBuy:  {levels: map[int64]*Level{}, prices: newPriceHeap(models.SideBuy)},
			models.SideSell: {levels: map[int64]*Level{}, prices: newPriceHeap(models.SideSell)},
		},
		nodes:           map[uuid.UUID]*node{},
		applied:         map[uuid.UUID]struct{}{},
		knownOrders:     map[uuid.UUID]struct{}{},
		publishedOffset: -1,
	}
}

func (b *Book) Len() int {
	return len(b.nodes)
}

func (b *Book) Get(orderID uuid.UUID) (*models.Order, bool) {
	node, ok := b.nodes[orderID]
	if !ok {
		return nil, false
	}

	return node.order, true
}

// The order goes last in its price level.
func (b *Book) Put(order *models.Order) {
	side := b.sides[order.Side]

	level, ok := side.levels[order.Price]
	if !ok {
		level = &Level{Price: order.Price}
		side.levels[order.Price] = level
		side.prices.pushPrice(order.Price)
	}

	b.nodes[order.ID] = level.append(order)
}

// Reports whether the order was in the book. An emptied level leaves the
// side at once; its price leaves the heap when it reaches the top.
func (b *Book) Remove(orderID uuid.UUID) bool {
	node, ok := b.nodes[orderID]
	if !ok {
		return false
	}

	level := node.level
	level.unlink(node)
	delete(b.nodes, orderID)

	if level.head == nil {
		delete(b.sides[node.order.Side].levels, level.Price)
	}

	return true
}

// Lowers what is pending of an order in place, so it keeps its place in its
// level. Reports whether the order was in the book.
func (b *Book) ReduceQuantity(orderID uuid.UUID, quantity int64) bool {
	node, ok := b.nodes[orderID]
	if !ok {
		return false
	}

	node.level.Volume -= node.order.Quantity - quantity
	node.order.Quantity = quantity

	return true
}

func (b *Book) GetLevel(orderSide models.Side, price int64) (*Level, bool) {
	level, ok := b.sides[orderSide].levels[price]

	return level, ok
}

// The highest buy or the lowest sell.
func (b *Book) GetBestLevel(orderSide models.Side) (*Level, bool) {
	side := b.sides[orderSide]

	for side.prices.Len() > 0 {
		if level, ok := side.levels[side.prices.getTop()]; ok {
			return level, true
		}

		side.prices.popPrice()
	}

	return nil, false
}

// From the best price to the worst.
func (b *Book) Levels(orderSide models.Side) iter.Seq[*Level] {
	side := b.sides[orderSide]

	prices := slices.SortedFunc(maps.Keys(side.levels), func(a, b int64) int {
		if orderSide == models.SideBuy {
			return cmp.Compare(b, a)
		}

		return cmp.Compare(a, b)
	})

	return func(yield func(*Level) bool) {
		for _, price := range prices {
			if !yield(side.levels[price]) {
				return
			}
		}
	}
}
