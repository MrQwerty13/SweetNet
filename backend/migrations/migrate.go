// Package migrations embeds the schema so the admin binary is self-contained.
package migrations

import (
	"context"
	"database/sql"
	"embed"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

func Up(ctx context.Context, databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
