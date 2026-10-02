package orderbook

import (
	"iter"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
)

// The orders of one price, in arrival order: their pending quantity and how many they are.
type Level struct {
	Price  int64
	Volume int64
	Count  int
	head   *node
	tail   *node
}

type node struct {
	order *models.Order
	level *Level
	prev  *node
	next  *node
}

func (l *Level) append(order *models.Order) *node {
	added := &node{order: order, level: l, prev: l.tail}

	if l.tail == nil {
		l.head = added
	} else {
		l.tail.next = added
	}

	l.tail = added
	l.Volume += order.Quantity
	l.Count++

	return added
}

func (l *Level) unlink(removed *node) {
	if removed.prev == nil {
		l.head = removed.next
	} else {
		removed.prev.next = removed.next
	}

	if removed.next == nil {
		l.tail = removed.prev
	} else {
		removed.next.prev = removed.prev
	}

	l.Volume -= removed.order.Quantity
	l.Count--
}

// The order that arrived first, which crosses before the others.
func (l *Level) GetFirst() *models.Order {
	return l.head.order
}

func (l *Level) Orders() iter.Seq[*models.Order] {
	return func(yield func(*models.Order) bool) {
		for current := l.head; current != nil; current = current.next {
			if !yield(current.order) {
				return
			}
		}
	}
}
