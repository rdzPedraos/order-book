package walletdb

import (
	"fmt"

	"gofr.dev/pkg/gofr"
	gofrSQL "gofr.dev/pkg/gofr/datasource/sql"
)

// Commits when apply succeeds and rolls back otherwise, so a use case either
// changes everything it touches or nothing.
func runInTransaction(ctx *gofr.Context, apply func(tx *gofrSQL.Tx) error) error {
	tx, err := ctx.SQL.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	if err := apply(tx); err != nil {
		_ = tx.Rollback()

		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}
