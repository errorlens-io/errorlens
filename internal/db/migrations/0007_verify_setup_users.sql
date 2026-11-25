-- Accounts created by first-run setup are verified by construction (their
-- creator had server access). Invited accounts proved ownership via the link.
UPDATE users SET email_verified_at = created_at
WHERE email_verified_at IS NULL AND (is_superuser OR oidc_subject IS NOT NULL);
