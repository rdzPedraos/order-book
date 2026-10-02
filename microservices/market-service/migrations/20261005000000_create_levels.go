package migrations

import "gofr.dev/pkg/gofr/migration"

// The book's levels as the engine last reported them. The key serves the
// reads, which take the best prices of one side of a book.
const createLevelsTable = `
CREATE TABLE IF NOT EXISTS levels (
	book   TEXT   NOT NULL,
	side   TEXT   NOT NULL CHECK (side IN ('BUY', 'SELL')),
	price  BIGINT NOT NULL,
	volume BIGINT NOT NULL,
	orders INT    NOT NULL,
	PRIMARY KEY (book, side, price)
)`

func createLevels() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createLevelsTable)

			return err
		},
	}
}
