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

	// Every command of the book goes to this partition of the log, so whoever
	// reads a book knows which partition to open.
	Partition int32
}

// Tickers are never reordered: "VIB-BRL" is unknown.
func Normalize(id string) (Book, error) {
	book, ok := registry[strings.ToUpper(id)]
	if !ok {
		return Book{}, ErrUnknownBook
	}

	return book, nil
}
