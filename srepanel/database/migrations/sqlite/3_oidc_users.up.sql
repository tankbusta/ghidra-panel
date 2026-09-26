CREATE TABLE oidc_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    issuer TEXT NOT NULL,
    subject TEXT NOT NULL,
    created_at INTEGER DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE UNIQUE INDEX idx_oidc_users_issuer_subject ON oidc_users (issuer, subject);
