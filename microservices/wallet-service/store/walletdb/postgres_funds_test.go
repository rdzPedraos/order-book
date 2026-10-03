package walletdb

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
)

type encodedBatch struct {
	operations []models.FundsOperation
}

func (batch encodedBatch) Match(value driver.Value) bool {
	encoded, isString := value.(string)
	if !isString {
		return false
	}

	var rows []fundsRow
	if err := json.Unmarshal([]byte(encoded), &rows); err != nil || len(rows) != len(batch.operations) {
		return false
	}

	for position, row := range rows {
		if !isRowOf(row, position, batch.operations[position]) {
			return false
		}
	}

	return true
}

func isRowOf(row fundsRow, position int, operation models.FundsOperation) bool {
	return row.Position == position && row.ID != uuid.Nil && !row.CreatedAt.IsZero() &&
		row.Type == operation.Type && row.MessageID == operation.MessageID && row.OrderID == operation.OrderID &&
		row.UserID == operation.UserID && row.Currency == operation.Currency && row.Amount == operation.Amount
}

func newResultRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"operation_position", "operation_result"})
}

func TestPostgresApplyFundsBatch(t *testing.T) {
	reserve := fundsOperation(models.MovementReserve, "user-a", 800)
	release := fundsOperation(models.MovementRelease, "user-a", 500)

	t.Run("a batch is one query and its results follow the batch order", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(applyFundsBatch).WithArgs(encodedBatch{[]models.FundsOperation{reserve, release}}).
			WillReturnRows(newResultRows().AddRow(0, "OK").AddRow(1, "release_exceeds_reservation"))

		results, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{reserve, release})
		c.NoError(err)
		c.Equal([]models.FundsResult{
			{MessageID: reserve.MessageID, Type: models.MovementReserve, Result: models.ResultOK},
			{MessageID: release.MessageID, Type: models.MovementRelease, Result: models.ResultReleaseExceedsReservation},
		}, results)
	})

	t.Run("database error is returned", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		connectionReset := errors.New("connection reset")
		mocks.SQL.ExpectQuery(applyFundsBatch).WillReturnError(connectionReset)

		_, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{reserve})
		c.ErrorIs(err, connectionReset)
	})

	t.Run("fewer results than operations is an error", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(applyFundsBatch).WillReturnRows(newResultRows().AddRow(0, "OK"))

		_, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{reserve, release})
		c.ErrorIs(err, errUnexpectedFundsResults)
	})

	t.Run("a result out of the batch order is an error", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(applyFundsBatch).WillReturnRows(newResultRows().AddRow(1, "OK"))

		_, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{reserve})
		c.ErrorIs(err, errUnexpectedFundsResults)
	})

	t.Run("result that cannot be read is an error", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(applyFundsBatch).WillReturnRows(newResultRows().AddRow("first", "OK"))

		_, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{reserve})
		c.ErrorContains(err, "scan funds result")
	})
}
