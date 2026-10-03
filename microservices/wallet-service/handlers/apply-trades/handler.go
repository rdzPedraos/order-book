// Package applytrades handles the batches of TradeExecuted events of
// orders.events: it moves each trade between buyer and seller, from what each
// one had reserved, and writes it in the ledger. main.go starts it in the
// trades role.
package applytrades

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

var Routes = []string{events.RouteTradeExecuted}

// A store failure is returned so the consumer retries the batch.
func Handle(ctx *gofr.Context, messages []events.Message) error {
	trades := buildTrades(ctx, messages)
	if len(trades) == 0 {
		return nil
	}

	return walletdb.ApplyTrades(ctx, trades)
}

// A trade that cannot be read never will, so it is logged and left out.
func buildTrades(ctx *gofr.Context, messages []events.Message) []models.Trade {
	trades := make([]models.Trade, 0, len(messages))

	for _, message := range messages {
		trade, err := buildTrade(message)
		if err != nil {
			ctx.Logger.Errorf("skipping trade %s: %v", message.ID, err)

			continue
		}

		trades = append(trades, *trade)
	}

	return trades
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
