# Security Policy

ErrorLens exists to keep sensitive data private, so we take security reports seriously.

## Reporting a vulnerability

**Please do not open a public issue.** Report privately via GitHub Security Advisories
("Report a vulnerability" on the repository's Security tab).

Include what you found, how to reproduce it, and the version/commit affected. We aim to
acknowledge reports within 3 business days and to ship a fix for confirmed high-severity
issues within 30 days. We will credit you in the release notes unless you prefer otherwise.

## Supported versions

Until 1.0, only the latest release receives security fixes.

## Design guarantees we treat as security bugs if broken

- Data from one organization is visible to another organization.
- Sensitive values reach the database despite matching an active scrubbing rule.
- ErrorLens sends any data to a third party without an administrator enabling it.
- The audit log can be modified or deleted through the application.
