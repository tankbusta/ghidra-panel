package database

import (
	"context"
)

// OIDCUserIDBase tags user IDs belonging to OIDC identities.
// Discord snowflakes will not reach bit 62 until ~2049.
const OIDCUserIDBase uint64 = 1 << 62

// GetOrCreateOIDCUserID returns a stable local user ID for an OIDC identity.
func (d *DB) GetOrCreateOIDCUserID(ctx context.Context, issuer, subject string) (uint64, error) {
	var rowID int64
	err := d.
		QueryRowContext(ctx,
			d.rebind(`INSERT INTO oidc_users (issuer, subject) VALUES (?, ?)
			ON CONFLICT (issuer, subject) DO UPDATE SET issuer = excluded.issuer
			RETURNING id`),
			issuer, subject,
		).
		Scan(&rowID)
	if err != nil {
		return 0, err
	}
	return OIDCUserIDBase | uint64(rowID), nil
}
