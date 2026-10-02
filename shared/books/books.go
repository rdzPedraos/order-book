// Package books is the registry of tradable books, configured in code.
//
// It is shared because the book id travels in requests, commands and events:
// every service must accept and validate a book the same way.
package books

import (
	"strings"

	"github.com/rdzpedraos/order-book/shared/fault"
	"github.com/rdzpedraos/order-book/shared/money"
)

var ErrUnknownBook = fault.New("unknown_book", "unknown book")

// Book is a tradable pair, as configured. Base is the traded asset and Quote
// the currency of its price; the ID alone does not say which is which.
type Book struct {
	ID    string
	Base  money.Currency
	Quote money.Currency
}

// Tickers are never reordered: "VIB-BRL" is unknown.
func Normalize(input string) (Book, error) {
	return lookup(strings.ToUpper(input))
}

func lookup(id string) (Book, error) {
	book, ok := registry[id]
	if !ok {
		return Book{}, ErrUnknownBook
	}

	return book, nil
}
