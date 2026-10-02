package events

import (
	"strconv"

	"github.com/google/uuid"
)

// What the matching engine publishes when orders cross.
const (
	RouteTradeExecuted         = "orders.events.TradeExecuted"
	RouteOrderBookLevelChanged = "orders.events.OrderBookLevelChanged"
)

// The remainder of an incoming order whose next counterparty is its own person.
const ReasonSelfTradePrevented = "self_trade_prevented"

// The header is the incoming order's. Price is the maker's, in the quote's
// minimal units per whole unit of the base; Amount is what the buyer pays.
// An order is fully executed when its trades add up to its quantity.
type TradeExecuted struct {
	EventHeader
	TradeID     uuid.UUID `json:"tradeId"`
	BuyOrderID  uuid.UUID `json:"buyOrderId"`
	SellOrderID uuid.UUID `json:"sellOrderId"`
	BuyerID     string    `json:"buyerId"`
	SellerID    string    `json:"sellerId"`
	MakerSide   string    `json:"makerSide"`
	Price       int64     `json:"price,string"`
	Quantity    int64     `json:"quantity,string"`
	Amount      int64     `json:"amount,string"`
}

// The whole state of one price level after a command: Volume is its pending
// quantity and Orders how many orders rest there; both are 0 when it emptied.
type OrderBookLevelChanged struct {
	EventHeader
	Side   string `json:"side"`
	Price  int64  `json:"price,string"`
	Volume int64  `json:"volume,string"`
	Orders int    `json:"orders"`
}

// Fixed by the command and the trade's position among its fills, like the id
// of an event, so a replay names the same trade. The "trade:" prefix keeps it
// apart from the ids of the command's events.
func NewTradeID(commandID uuid.UUID, fillIndex int) uuid.UUID {
	return uuid.NewSHA1(commandID, []byte("trade:"+strconv.Itoa(fillIndex)))
}
