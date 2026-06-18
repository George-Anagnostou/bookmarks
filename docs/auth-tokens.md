# Per-Device Tokens and Auth Plan

This document plans the next authentication step for the bookmark manager. The current alpha uses one shared bearer token from `BOOKMARKS_TOKEN`. That is enough to prove the product works, but it makes revocation and device management awkward.

The recommended next step is DB-backed per-device bearer tokens.

## Goals

- Keep saving bookmarks frictionless from iPhone Shortcuts, CLI clients, curl, and future scripts.
- Avoid third-party auth services and avoid new Go dependencies.
- Allow separate named credentials per device, such as `iphone`, `macbook`, `android`, and `work-laptop`.
- Allow revoking one device without rotating credentials everywhere.
- Store only token hashes in SQLite.
- Preserve the existing `Authorization: Bearer <token>` API contract for clients.
- Leave room for future sharing without forcing multi-user auth into the project too early.

## Non-Goals

- No OAuth for now.
- No browser login/session system for this workstream.
- No mTLS/client certificates for now. They are secure, but setup friction is high on phones and casual devices.
- No public self-service account creation.
- No role system until sharing becomes real.

## Current State

The server is configured with one env token:

```text
BOOKMARKS_TOKEN=<secret>
```

The server accepts requests with:

```text
Authorization: Bearer <secret>
```

`internal/server` compares that bearer token to the env token with constant-time comparison.

This is simple and effective, but it has two operational problems:

- Every device shares the same secret.
- Rotating a leaked token breaks every configured client at once.

## Recommended Design

Keep bearer tokens, but make them database records.

Each token has:

- A public token id used for lookup.
- A random secret shown only once at creation time.
- A hash of that secret stored in SQLite.
- A human name, such as `iphone-shortcut`.
- Timestamps for creation, last use, and revocation.

The client still sends one opaque token string:

```text
Authorization: Bearer bm1_<token-id>_<secret>
```

Example shape:

```text
bm1_Zr6RmcBYAvxY0Q_E82GvqZDXo5KbVP7Dy9GG9fqFIvvklVdyxGlt7DJk
```

The `bm1` prefix gives us a version marker. The token id is not secret; it lets the server find the token row quickly. The secret is the actual credential.

## Token Generation

Use the standard library:

- `crypto/rand` for randomness.
- `encoding/base64.RawURLEncoding` for URL-safe token text.
- `crypto/sha256` for hashing.
- `crypto/subtle` for constant-time hash comparison.

Suggested sizes:

- Token id: 12 to 16 random bytes before base64 encoding.
- Secret: 32 random bytes before base64 encoding.

The generated token is shown once. After that, only the hash remains in the database.

## Token Hashing

Store a SHA-256 hash of the secret component, not the full bearer token.

Suggested helper behavior:

```text
parse "bm1_<token-id>_<secret>"
lookup row by token_id
sha256(secret)
constant-time compare computed hash against stored hash
reject if revoked_at is not null
update last_used_at asynchronously or cheaply after successful auth
```

Because the secret is generated from 32 random bytes, a plain SHA-256 hash is acceptable. Password hashing algorithms are important for human-memorable passwords; these tokens are high-entropy random secrets.

An optional future hardening step is to add a server-side pepper:

```text
BOOKMARKS_TOKEN_PEPPER=<server-secret>
```

Then hash `pepper || secret`. That protects token hashes if only the database leaks, but it adds one more secret to manage. I would skip the pepper initially and rely on high-entropy generated tokens.

## SQLite Schema

Add a table like this:

```sql
CREATE TABLE IF NOT EXISTS auth_tokens (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  secret_hash BLOB NOT NULL,
  created_at TEXT NOT NULL,
  last_used_at TEXT,
  revoked_at TEXT
) STRICT;

CREATE INDEX IF NOT EXISTS auth_tokens_revoked_at_idx
ON auth_tokens(revoked_at);
```

Notes:

- `id` is the public token id embedded in the bearer token.
- `name` is a human label for management.
- `secret_hash` should be raw 32-byte SHA-256 output, not hex text, unless hex is materially easier during development.
- `revoked_at IS NULL` means active.
- `last_used_at` is useful for cleanup and audits.

If we later support sharing, this table can gain an `owner_id` or `principal_id`. For now, a single-user table is enough.

## Store Interfaces

Avoid forcing token auth into the bookmark domain interface. Add a separate auth-focused store interface near the server or in a new internal package.

Possible package:

```text
internal/auth
```

Possible types:

```go
type Token struct {
    ID         string
    Name       string
    SecretHash []byte
    CreatedAt  time.Time
    LastUsedAt *time.Time
    RevokedAt  *time.Time
}

type Store interface {
    CreateToken(ctx context.Context, name string) (PlainToken, Token, error)
    TokenByID(ctx context.Context, id string) (Token, error)
    ListTokens(ctx context.Context) ([]Token, error)
    RevokeToken(ctx context.Context, name string) error
    MarkTokenUsed(ctx context.Context, id string, usedAt time.Time) error
}
```

`PlainToken` should contain the one-time bearer token string. It should not be stored.

An alternative is to keep the auth methods on `bookmarks.SQLStore` directly. That is less pure, but acceptable for this small app. If doing that, define a small server-facing interface so handlers do not depend on the concrete SQL store.

## Server Middleware Changes

Current server config:

```go
type Config struct {
    Store bookmarks.Store
    Token string
}
```

Future server config:

```go
type Config struct {
    Store     bookmarks.Store
    AuthStore auth.Store
    Token     string // temporary fallback during migration
}
```

Middleware flow:

1. Extract bearer token from `Authorization`.
2. If token has `bm1_` format:
   - Parse token id and secret.
   - Load token row by id.
   - Reject missing or revoked token.
   - Hash secret and constant-time compare with stored hash.
   - Optionally mark `last_used_at`.
3. During migration only, fall back to comparing against `BOOKMARKS_TOKEN`.
4. Return `401` on failure.

After migration, remove the env-token fallback and make `AuthStore` required.

## Migration From Single Env Token

Do this in two phases so existing clients do not break.

Phase 1:

- Add `auth_tokens` table.
- Add DB token validation.
- Keep `BOOKMARKS_TOKEN` as a fallback.
- Add admin command to create named tokens.
- Update each device to use its new token.

Phase 2:

- Add logging or startup warning when `BOOKMARKS_TOKEN` fallback is configured.
- Remove fallback after all clients have moved.
- Eventually replace `BOOKMARKS_TOKEN` with an admin bootstrap mechanism.

This keeps the iPhone Shortcut and CLI working during rollout.

## Admin UX

Prefer local CLI administration over HTTP admin endpoints at first. Admin endpoints would need their own stronger protection, and this app already runs on your VPS where local commands are available.

Proposed server-side admin CLI:

```text
bookmarkd token create iphone
bookmarkd token list
bookmarkd token revoke iphone
```

The create command prints the full bearer token once:

```text
created token iphone
token: bm1_Zr6RmcBYAvxY0Q_E82GvqZDXo5KbVP7Dy9GG9fqFIvvklVdyxGlt7DJk
```

The list command should not print secrets:

```text
NAME             ID                CREATED_AT            LAST_USED_AT          REVOKED
iphone           Zr6RmcBYAvxY0Q    2026-06-18T10:00:00Z  2026-06-18T10:12:30Z no
macbook          hTM_E2iylgafPQ    2026-06-18T10:05:00Z                       no
old-android      Q0TBbCVcd8Dp0Q    2026-06-18T10:07:00Z                       yes
```

For the client CLI, no change is required initially:

```text
BOOKMARKS_TOKEN=<per-device-token>
bookmarkctl add https://example.com
```

Later, `bookmarkctl login` could store a token in a local config file, but env vars are enough for the next iteration.

## Adding Devices

Initial flow:

1. SSH into the VPS.
2. Run `bookmarkd token create iphone`.
3. Copy the printed token into the iPhone Shortcut.
4. Confirm `last_used_at` updates after the first save.

This is not zero friction, but it is explicit and secure enough for a private tool.

Future easier flow:

- Add a short-lived pairing code command:

```text
bookmarkd pair create iphone
```

It would print a one-time code valid for a few minutes. A device could exchange that code for a token. This is more complex and should wait until per-device tokens are working.

## Sharing Considerations

Per-device tokens are also a stepping stone toward sharing, but they are not yet full multi-user auth.

If sharing becomes important, there are two likely paths:

1. Named access tokens with scopes:
   - `read`
   - `write`
   - `admin`
   - optional collection/tag restrictions

2. User/principal model:
   - `principals`
   - `auth_tokens.principal_id`
   - bookmark ownership or shared collections

For now, do not add scopes unless there is a concrete sharing feature. It is enough to make the schema easy to extend later.

Possible future schema:

```sql
ALTER TABLE auth_tokens ADD COLUMN scopes TEXT NOT NULL DEFAULT 'read,write';
```

For SQLite, storing scopes as comma-separated text is acceptable early on. A normalized `token_scopes` table can wait until the need is real.

## Threat Model

This feature is designed for a private bookmark server behind HTTPS.

Risks addressed:

- A lost device can be revoked individually.
- A leaked SQLite database does not expose usable bearer tokens.
- Tokens are high entropy and not guessable.
- Token comparison does not leak partial matches.
- The API contract stays compatible with iOS Shortcuts and simple clients.

Risks not fully addressed:

- If a device token is copied from the device, it can be used until revoked.
- If the VPS is fully compromised, all tokens can be accepted or replaced by the attacker.
- If HTTPS is misconfigured, bearer tokens can leak in transit.
- If logs capture `Authorization`, tokens can leak. Do not log auth headers.

Operational rules:

- Always use HTTPS outside localhost.
- Never log bearer tokens.
- Print generated tokens only once.
- Use per-device token names that make revocation obvious.
- Revoke old tokens instead of deleting rows, at least initially, so audit data remains available.

## Implementation Tickets

### Ticket 1: Add Token Generation Helpers

Build helpers that generate and parse `bm1_<id>_<secret>` tokens.

Acceptance criteria:

- Generated token id and secret use `crypto/rand`.
- Token strings parse back into version, id, and secret.
- Invalid token strings are rejected.
- Secret hash uses SHA-256.
- Tests cover valid tokens, malformed tokens, wrong version, empty id, and empty secret.

### Ticket 2: Add Auth Token Schema

Add the `auth_tokens` table to SQLite schema setup.

Acceptance criteria:

- New databases include `auth_tokens`.
- Existing databases get the table on startup.
- Schema uses `STRICT`.
- Token names are unique.
- Tests verify token rows can be inserted, loaded, listed, revoked, and marked used.

### Ticket 3: Add Auth Store Methods

Implement token storage operations.

Acceptance criteria:

- Creating a token stores only the hash, not the plain token.
- Listing tokens never returns secrets.
- Revoking a token sets `revoked_at`.
- Revoked tokens fail validation.
- `last_used_at` can be updated after successful validation.

### Ticket 4: Update Server Auth Middleware

Teach `requireAuth` to validate DB-backed tokens.

Acceptance criteria:

- Requests with valid DB tokens succeed.
- Requests with invalid, unknown, or revoked tokens return `401`.
- Existing `BOOKMARKS_TOKEN` still works during migration.
- Constant-time comparison is used for secret hash checks.
- Tests cover both new tokens and legacy env fallback.

### Ticket 5: Add Admin Token Commands

Add local admin commands for token management.

Acceptance criteria:

- `bookmarkd token create <name>` prints the full token once.
- `bookmarkd token list` lists names, ids, created time, last used time, and revoked status.
- `bookmarkd token revoke <name>` revokes a token.
- Commands use `BOOKMARKS_DBPATH` to locate the database.
- Commands do not require `BOOKMARKS_TOKEN`.
- Tests cover create/list/revoke command behavior.

### Ticket 6: Migrate Devices

Move real clients from the global env token to per-device tokens.

Acceptance criteria:

- iPhone Shortcut uses its own named token.
- `bookmarkctl` on each laptop uses its own named token.
- `bookmarkd token list` shows recent `last_used_at` values.
- The legacy `BOOKMARKS_TOKEN` fallback is still available until all clients are migrated.

### Ticket 7: Remove Legacy Token Fallback

After all devices use DB tokens, remove the global token requirement.

Acceptance criteria:

- `BOOKMARKS_TOKEN` is no longer required for normal server startup.
- Server startup requires an auth token table to exist or creates it automatically.
- Requests with the old global token fail.
- Deployment docs are updated to describe token creation instead of a single shared env token.

## Recommended Next Step

Start with token generation and parsing helpers. That isolates the trickiest security-sensitive logic into small tests before touching server middleware or SQLite.
