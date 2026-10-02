package orderbook

import (
	"container/heap"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
)

// A max-heap of buy prices or a min-heap of sell prices. A price whose level
// emptied stays until it reaches the top, where the book discards it.
type priceHeap struct {
	prices   []int64
	isBetter func(a, b int64) bool
}

func newPriceHeap(side models.Side) *priceHeap {
	if side == models.SideBuy {
		return &priceHeap{isBetter: func(a, b int64) bool { return a > b }}
	}

	return &priceHeap{isBetter: func(a, b int64) bool { return a < b }}
}

func (h *priceHeap) pushPrice(price int64) { heap.Push(h, price) }

func (h *priceHeap) popPrice() { heap.Pop(h) }

func (h *priceHeap) getTop() int64 { return h.prices[0] }

func (h *priceHeap) Len() int { return len(h.prices) }

func (h *priceHeap) Less(i, j int) bool { return h.isBetter(h.prices[i], h.prices[j]) }

func (h *priceHeap) Swap(i, j int) { h.prices[i], h.prices[j] = h.prices[j], h.prices[i] }

func (h *priceHeap) Push(value any) { h.prices = append(h.prices, value.(int64)) }

func (h *priceHeap) Pop() any {
	last := h.prices[len(h.prices)-1]
	h.prices = h.prices[:len(h.prices)-1]

	return last
}
