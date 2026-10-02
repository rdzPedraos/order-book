// Package applytrade handles the TradeExecuted events of orders.events: it
// moves each trade between buyer and seller, from what each one had reserved,
// and writes it in the ledger. main.go subscribes it in the trades role.
package applytrade

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// A store failure is returned so the consumer retries the trade; a payload
// that cannot be read is logged and skipped.
func Handle(ctx *gofr.Context, message events.Message) error {
	trade, err := buildTrade(message)
	if err != nil {
		ctx.Logger.Errorf("skipping trade %s: %v", message.ID, err)

		return nil
	}

	return walletdb.ApplyTrade(ctx, *trade)
}

func buildTrade(message events.Message) (*models.Trade, error) {
	var executed events.TradeExecuted
	if err := message.ParsePayload(&executed); err != nil {
		return nil, err
	}

	book, err := books.Normalize(message.Book)
	if err != nil {
		return nil, err
	}

	return &models.Trade{
		MessageID: message.ID, BuyOrderID: executed.BuyOrderID, SellOrderID: executed.SellOrderID,
		BuyerID: executed.BuyerID, SellerID: executed.SellerID, Base: book.Base, Quote: book.Quote,
		Quantity: executed.Quantity, Amount: executed.Amount, CreatedAt: message.CreatedAt,
	}, nil
}
