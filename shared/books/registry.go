package books

import "github.com/rdzpedraos/order-book/shared/money"

var registry = map[string]Book{
	"BRL-VIB": {ID: "BRL-VIB", Base: money.VIB, Quote: money.BRL},
}
