package walletdb

import (
	"fmt"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
)

const getBalances = `
SELECT user_id, currency, available, reserved
FROM balances
WHERE user_id = $1`

func (postgres) getBalances(ctx *gofr.Context, userID string) ([]models.Balance, error) {
	rows, err := ctx.SQL.QueryContext(ctx, getBalances, userID)
	if err != nil {
		return nil, fmt.Errorf("get balances: %w", err)
	}
	defer rows.Close()

	var balances []models.Balance

	for rows.Next() {
		var balance models.Balance
		if err := rows.Scan(&balance.UserID, &balance.Currency, &balance.Available, &balance.Reserved); err != nil {
			return nil, fmt.Errorf("scan balance: %w", err)
		}

		balances = append(balances, balance)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get balances: %w", err)
	}

	return balances, nil
}
