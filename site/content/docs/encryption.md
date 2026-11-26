---
title: Encryption
description: Encrypt every stored ErrorLens event with AES-256-GCM using your own key: how to enable it, rotate keys, back them up and what stays searchable.
section: Privacy & security
order: 2
seo_title: Encryption of Stored Events
---

## In transit

All traffic to ErrorLens uses HTTPS. On errorlens.io, connections are encrypted from your SDK or browser to Cloudflare's edge, and again from Cloudflare to the ErrorLens servers, with certificates verified at each hop.

## At rest

ErrorLens can encrypt the full payload of every stored event — stack traces, variables, request data, breadcrumbs — with **AES-256-GCM**:

- Each organization has its own data key. Data keys are themselves encrypted with a master key that is kept outside the database.
- Each event is bound to its own ID, so an encrypted payload can't be copied onto another event.
- Someone who obtains a database backup without the master key can see issue titles and counts, but not the contents of events.

**On errorlens.io, encryption at rest is always on.**

## Self-hosted

Set a 32-byte key, base64-encoded, to turn encryption on:

```bash
openssl rand -base64 32
```

```bash
ERRORLENS_ENCRYPTION_KEY="<the generated key>"
```

Events received afterwards are encrypted; existing events stay readable as they are.

> **Back up the key.** It is the only way to read encrypted events. If it is lost, they cannot be recovered — by you or by anyone else.

Issue titles, culprits, tags and counts stay unencrypted so you can still search, sort and filter issues.
