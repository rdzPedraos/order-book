//go:build integration

package walletdb

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/config"
	"gofr.dev/pkg/gofr/container"
	"gofr.dev/pkg/gofr/migration"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/migrations"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/shared/money"
)

func newIntegrationContext(t *testing.T) *gofr.Context {
	t.Helper()
	c := require.New(t)

	db := container.NewContainer(config.NewMockConfig(map[string]string{
		"DB_DIALECT": "postgres", "DB_HOST": "localhost", "DB_PORT": "5432", "DB_USER": "exchange",
		"DB_PASSWORD": "exchange", "DB_NAME": "wallet_service", "DB_SSL_MODE": "disable",
	}))
	c.NotNil(db.SQL, "could not connect to PostgreSQL; run docker compose -f deploy/docker-compose.yml up -d")

	migration.Run(migrations.All(), db)

	return &gofr.Context{Context: context.Background(), Container: db}
}

func newUserID() string {
	return "integration-" + uuid.NewString()
}

func TestGetBalancesIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("stored balance is read", func(t *testing.T) {
		c := require.New(t)
		userID := newUserID()
		_, err := ctx.SQL.ExecContext(ctx, `INSERT INTO balances (user_id, currency, available, reserved) VALUES ($1, 'BRL', 100, 900)`, userID)
		c.NoError(err)

		balances, err := GetBalances(ctx, userID)
		c.NoError(err)
		c.Equal([]models.Balance{
			{UserID: userID, Currency: money.BRL, Available: 100, Reserved: 900},
			{UserID: userID, Currency: money.VIB},
		}, balances)
	})

	t.Run("wallet without rows is zero", func(t *testing.T) {
		c := require.New(t)
		userID := newUserID()

		balances, err := GetBalances(ctx, userID)
		c.NoError(err)
		c.Equal([]models.Balance{{UserID: userID, Currency: money.BRL}, {UserID: userID, Currency: money.VIB}}, balances)
	})
}

func TestDepositIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("deposits add up and leave their movements", func(t *testing.T) {
		c := require.New(t)
		userID := newUserID()

		c.NoError(Deposit(ctx, deposit(userID, money.BRL, 15000)))
		c.NoError(Deposit(ctx, deposit(userID, money.BRL, 500)))

		balances, err := GetBalances(ctx, userID)
		c.NoError(err)
		c.Equal(int64(15500), balances[0].Available)

		var movements int
		c.NoError(ctx.SQL.QueryRowContext(ctx, `SELECT count(*) FROM ledger WHERE user_id = $1 AND type = 'DEPOSIT'`, userID).Scan(&movements))
		c.Equal(2, movements)
	})
}

func TestWithdrawIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("only the available balance can be withdrawn", func(t *testing.T) {
		c := require.New(t)
		userID := newUserID()
		c.NoError(Deposit(ctx, deposit(userID, money.BRL, 10000)))

		c.NoError(Withdraw(ctx, withdrawal(userID, 4000)))
		c.ErrorIs(Withdraw(ctx, withdrawal(userID, 6001)), models.ErrInsufficientFunds)

		balances, err := GetBalances(ctx, userID)
		c.NoError(err)
		c.Equal(int64(6000), balances[0].Available)

		var movements int
		c.NoError(ctx.SQL.QueryRowContext(ctx, `SELECT count(*) FROM ledger WHERE user_id = $1 AND type = 'WITHDRAWAL'`, userID).Scan(&movements))
		c.Equal(1, movements)
	})
}

func getSignedAmount(movement models.Movement) int64 {
	if movement.Type == models.MovementWithdrawal {
		return -movement.Amount
	}

	if movement.Type == models.MovementDeposit {
		return movement.Amount
	}

	return 0
}

func TestListMovementsIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("balance rebuilt from the movements", func(t *testing.T) {
		c := require.New(t)
		userID := newUserID()
		c.NoError(Deposit(ctx, deposit(userID, money.BRL, 10000)))
		c.NoError(Deposit(ctx, deposit(userID, money.BRL, 2500)))
		c.NoError(Withdraw(ctx, withdrawal(userID, 4000)))

		movements, err := ListMovements(ctx, MovementQuery{UserID: userID, Limit: 100})
		c.NoError(err)
		c.Len(movements, 3)

		var total int64
		for _, movement := range movements {
			total += getSignedAmount(movement)
		}

		balances, err := GetBalances(ctx, userID)
		c.NoError(err)
		c.Equal(balances[0].GetTotal(), total)
	})

	t.Run("newest first, filtered by date and paginated", func(t *testing.T) {
		c := require.New(t)
		userID := newUserID()
		older, newer := deposit(userID, money.BRL, 1), deposit(userID, money.BRL, 2)
		newer.CreatedAt = older.CreatedAt.Add(time.Hour)
		c.NoError(Deposit(ctx, older))
		c.NoError(Deposit(ctx, newer))

		page, err := ListMovements(ctx, MovementQuery{UserID: userID, Limit: 1})
		c.NoError(err)
		c.Equal(newer.ID, page[0].ID)

		next, err := ListMovements(ctx, MovementQuery{UserID: userID, Cursor: &page[0].ID, Limit: 1})
		c.NoError(err)
		c.Equal(older.ID, next[0].ID)

		from := older.CreatedAt.Add(time.Minute)
		inRange, err := ListMovements(ctx, MovementQuery{UserID: userID, From: &from, Limit: 10})
		c.NoError(err)
		c.Len(inRange, 1)
	})
}

func applyOne(c *require.Assertions, ctx *gofr.Context, operation models.FundsOperation) models.Result {
	results, err := ApplyFundsBatch(ctx, []models.FundsOperation{operation})
	c.NoError(err)

	return results[0].Result
}

func TestApplyFundsBatchIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("a repeated message moves funds once, and a rejection stays rejected", func(t *testing.T) {
		c := require.New(t)
		userID := newUserID()
		c.NoError(Deposit(ctx, deposit(userID, money.BRL, 1000)))
		reserve, tooMuch := fundsOperation(models.MovementReserve, userID, 800), fundsOperation(models.MovementReserve, userID, 500)

		c.Equal(models.ResultOK, applyOne(c, ctx, reserve))
		c.Equal(models.ResultInsufficientFunds, applyOne(c, ctx, tooMuch))
		c.Equal(models.ResultOK, applyOne(c, ctx, reserve))

		c.NoError(Deposit(ctx, deposit(userID, money.BRL, 5000)))
		c.Equal(models.ResultInsufficientFunds, applyOne(c, ctx, tooMuch), "a repeated message gets its first result")

		balances, err := GetBalances(ctx, userID)
		c.NoError(err)
		c.Equal(models.Balance{UserID: userID, Currency: money.BRL, Available: 5200, Reserved: 800}, balances[0])
	})

	t.Run("a release moves funds once and never beyond the reserved balance", func(t *testing.T) {
		c := require.New(t)
		userID := newUserID()
		c.NoError(Deposit(ctx, deposit(userID, money.BRL, 1000)))
		c.Equal(models.ResultOK, applyOne(c, ctx, fundsOperation(models.MovementReserve, userID, 800)))
		release := fundsOperation(models.MovementRelease, userID, 500)

		c.Equal(models.ResultOK, applyOne(c, ctx, release))
		c.Equal(models.ResultOK, applyOne(c, ctx, release))
		c.Equal(models.ResultReleaseExceedsReservation, applyOne(c, ctx, fundsOperation(models.MovementRelease, userID, 301)))

		balances, err := GetBalances(ctx, userID)
		c.NoError(err)
		c.Equal(models.Balance{UserID: userID, Currency: money.BRL, Available: 700, Reserved: 300}, balances[0])
	})
}

// Each goroutine runs its own operation at the same time against the same row.
func runConcurrently(apply ...func() models.Result) []models.Result {
	results := make([]models.Result, len(apply))

	var wg sync.WaitGroup
	for i, run := range apply {
		wg.Go(func() { results[i] = run() })
	}

	wg.Wait()

	return results
}

func countOK(results []models.Result) int {
	return len(slices.DeleteFunc(slices.Clone(results), func(result models.Result) bool { return result != models.ResultOK }))
}

func TestConcurrencyIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("non-negative invariant under concurrency", func(t *testing.T) {
		c := require.New(t)
		userID := newUserID()
		c.NoError(Deposit(ctx, deposit(userID, money.BRL, 100000)))
		first, second := fundsOperation(models.MovementReserve, userID, 80000), fundsOperation(models.MovementReserve, userID, 80000)

		results := runConcurrently(
			func() models.Result { return applyOne(c, ctx, first) },
			func() models.Result { return applyOne(c, ctx, second) },
		)

		c.Equal(1, countOK(results))
		c.Contains(results, models.ResultInsufficientFunds)

		balances, err := GetBalances(ctx, userID)
		c.NoError(err)
		c.Equal(models.Balance{UserID: userID, Currency: money.BRL, Available: 20000, Reserved: 80000}, balances[0])
	})

	t.Run("reservation and withdrawal at the same time", func(t *testing.T) {
		c := require.New(t)
		userID := newUserID()
		c.NoError(Deposit(ctx, deposit(userID, money.BRL, 100000)))
		reservation := fundsOperation(models.MovementReserve, userID, 80000)

		results := runConcurrently(
			func() models.Result { return applyOne(c, ctx, reservation) },
			func() models.Result {
				if err := Withdraw(ctx, withdrawal(userID, 80000)); err != nil {
					return models.ResultInsufficientFunds
				}

				return models.ResultOK
			},
		)

		c.Equal(1, countOK(results))

		balances, err := GetBalances(ctx, userID)
		c.NoError(err)
		c.Equal(int64(20000), balances[0].Available)
		c.GreaterOrEqual(balances[0].Reserved, int64(0))
	})
}
