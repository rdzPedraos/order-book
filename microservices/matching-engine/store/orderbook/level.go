package orderbook

import (
	"iter"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
)

// The orders of one price, in arrival order, and their pending quantity.
type Level struct {
	Price  int64
	Volume int64
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
