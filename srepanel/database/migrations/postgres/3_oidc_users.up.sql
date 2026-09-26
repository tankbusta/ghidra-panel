CREATE TABLE oidc_users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    issuer TEXT NOT NULL,
    subject TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE UNIQUE INDEX idx_oidc_users_issuer_subject ON oidc_users (issuer, subject);
