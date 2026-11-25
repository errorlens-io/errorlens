-- Phase 3: hosted mode (plans, quotas, usage), account security.

ALTER TABLE organizations
    ADD COLUMN plan                 text NOT NULL DEFAULT 'self-hosted',
    ADD COLUMN monthly_event_quota  bigint,          -- NULL = unlimited
    ADD COLUMN billing_customer_id  text NOT NULL DEFAULT '',
    ADD COLUMN billing_status       text NOT NULL DEFAULT '';

-- Daily usage per organization, used for quotas, the usage page and billing.
CREATE TABLE usage_daily (
    org_id          uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    day             date NOT NULL,
    events_accepted bigint NOT NULL DEFAULT 0,
    events_dropped  bigint NOT NULL DEFAULT 0, -- over quota or rate limited
    logs_accepted   bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (org_id, day)
);
ALTER TABLE usage_daily ENABLE ROW LEVEL SECURITY;
CREATE POLICY usage_daily_tenant ON usage_daily FOR SELECT USING (org_id = app_org_id());
GRANT SELECT ON usage_daily TO rootcause_app;

ALTER TABLE users
    ADD COLUMN email_verified_at timestamptz,
    ADD COLUMN totp_secret       bytea,      -- set once 2FA is confirmed
    ADD COLUMN totp_pending      bytea,      -- secret awaiting first valid code
    ADD COLUMN recovery_codes    text[] NOT NULL DEFAULT '{}', -- SHA-256 hex of unused codes
    ADD COLUMN oidc_subject      text;
CREATE UNIQUE INDEX users_oidc_subject_key ON users (oidc_subject) WHERE oidc_subject IS NOT NULL;

-- Existing accounts (created by setup or invitation) count as verified.
UPDATE users SET email_verified_at = created_at;

-- One-time tokens: email verification, password reset, 2FA login step.
CREATE TABLE user_tokens (
    token_hash bytea PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose    text NOT NULL CHECK (purpose IN ('verify_email', 'reset_password', 'mfa_login')),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX user_tokens_user_idx ON user_tokens (user_id);
