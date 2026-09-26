package database

import (
	"context"
	"database/sql"
	"errors"
	"go.mkw.re/ghidra-panel/common"
)

func (d *DB) GetRepository(ctx context.Context, name string) (*common.Repository, error) {
	repo := common.Repository{Name: name}
	err := d.
		QueryRowContext(ctx, d.rebind("SELECT webhook_url FROM repositories WHERE name = ?"), name).
		Scan(&repo.WebhookURL)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return &repo, nil
}

func (d *DB) SetRepositoryWebhook(ctx context.Context, name, url string) error {
	_, err := d.ExecContext(
		ctx,
		d.rebind(`INSERT INTO repositories (name, webhook_url) VALUES (?, ?)
		 ON CONFLICT(name) DO UPDATE SET webhook_url = ?, updated_at = CURRENT_TIMESTAMP`),
		name, url, url,
	)
	return err
}
