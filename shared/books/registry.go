package books

import "github.com/rdzpedraos/order-book/shared/money"

// Built on each call to avoid a mutable package-level map.
func registry() map[string]Book {
	return map[string]Book{
		"BRL-VIB": {ID: "BRL-VIB", Base: money.VIB, Quote: money.BRL},
	}
}
