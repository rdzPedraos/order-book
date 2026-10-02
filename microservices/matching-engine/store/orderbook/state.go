package orderbook

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

// One state per book, created on first use. The engine applies each book's
// commands from a single goroutine; the lock guards only the map of books.
var (
	booksLock sync.Mutex
	books     = map[string]*Book{}
)

func GetBook(bookID string) *Book {
	booksLock.Lock()
	defer booksLock.Unlock()

	book, ok := books[bookID]
	if !ok {
		book = New()
		books[bookID] = book
	}

	return book
}

// Starts every book empty until the test ends.
func InitEmpty(t testing.TB) {
	booksLock.Lock()
	previous := books
	books = map[string]*Book{}
	booksLock.Unlock()

	t.Cleanup(func() {
		booksLock.Lock()
		books = previous
		booksLock.Unlock()
	})
}

func (b *Book) GetSequence() uint64 {
	return b.sequence
}

func (b *Book) NextSequence() uint64 {
	b.sequence++

	return b.sequence
}

func (b *Book) IsApplied(messageID uuid.UUID) bool {
	_, ok := b.applied[messageID]

	return ok
}

func (b *Book) MarkApplied(messageID uuid.UUID) {
	b.applied[messageID] = struct{}{}
}

func (b *Book) IsKnownOrder(orderID uuid.UUID) bool {
	_, ok := b.knownOrders[orderID]

	return ok
}

func (b *Book) MarkKnownOrder(orderID uuid.UUID) {
	b.knownOrders[orderID] = struct{}{}
}

// After a restart, the events up to this command and index are already in
// the log, so rebuilding the book must not publish them again.
func (b *Book) SetPublishedUpTo(commandOffset int64, index int) {
	b.publishedOffset, b.publishedIndex = commandOffset, index
}

func (b *Book) IsPublished(commandOffset int64, index int) bool {
	return commandOffset < b.publishedOffset || (commandOffset == b.publishedOffset && index <= b.publishedIndex)
}
