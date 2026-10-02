package models

// One price level of a book's side: Volume is its pending quantity in the
// book's base, Price is in its quote, and Orders is how many orders rest there.
type Level struct {
	Book   string
	Side   string
	Price  int64
	Volume int64
	Orders int
}
