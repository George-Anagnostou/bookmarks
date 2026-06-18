# Edit and Delete CRUD Plan

This plan adds hard-delete and edit support to the existing bookmark manager.
The first version should stay small, explicit, and test-driven.

## Goals

- Add edit support for bookmark metadata and URL.
- Add hard delete support.
- Keep bearer-token authentication unchanged.
- Keep the API JSON-first.
- Keep the CLI standard-library-only.
- Preserve the existing package boundaries:
  - `internal/bookmarks`: domain types, validation, store contract, SQLite implementation.
  - `internal/server`: HTTP routes and request/response mapping.
  - `internal/apiclient`: typed HTTP client library.
  - `cmd/bookmarkctl`: CLI parsing, config loading, and output.

## Non-Goals

- Soft delete or archive behavior.
- Multi-user permissions.
- Token management.
- HTML UI.
- Full tag editing, unless tag storage is wired first.

## Design Choice: Address Bookmarks By ID

Edit and delete should target bookmark IDs:

```sh
bookmarkctl edit <id> -title "New title"
bookmarkctl delete <id>
```

This avoids ambiguity with URLs and avoids putting encoded URLs into route paths.
Because the current CLI list output does not show IDs, add a small prerequisite:

```sh
bookmarkctl list -ids
```

Suggested output:

```text
<id>\t<url>\t<title>
```

The existing `bookmarkctl list` output can remain:

```text
<url>\t<title>
```

## API Contract

### Edit Bookmark

```http
PATCH /api/bookmarks/{id}
Authorization: Bearer <token>
Content-Type: application/json
```

Request body:

```json
{
  "url": "https://example.com/new-url",
  "title": "Updated title",
  "notes": "Updated notes",
  "source": "bookmarkctl"
}
```

All fields are optional, but at least one field must be present.

Response:

```http
200 OK
Content-Type: application/json
```

```json
{
  "bookmark": {
    "id": "abc123",
    "url": "https://example.com/new-url",
    "normalized_url": "https://example.com/new-url",
    "title": "Updated title",
    "notes": "Updated notes",
    "source": "bookmarkctl",
    "created_at": "2026-06-18T12:00:00Z",
    "updated_at": "2026-06-18T12:05:00Z"
  }
}
```

Status mapping:

- `200 OK`: bookmark updated.
- `400 Bad Request`: bad JSON, empty update body, invalid URL, unsupported URL scheme, URL credentials, or missing URL host.
- `401 Unauthorized`: missing or invalid bearer token.
- `404 Not Found`: bookmark ID does not exist.
- `409 Conflict`: updated URL normalizes to an existing bookmark owned by a different ID.
- `413 Request Entity Too Large`: request body exceeds the configured limit.
- `500 Internal Server Error`: unexpected store/server failure.

### Delete Bookmark

```http
DELETE /api/bookmarks/{id}
Authorization: Bearer <token>
```

Response:

```http
204 No Content
```

Status mapping:

- `204 No Content`: bookmark deleted.
- `401 Unauthorized`: missing or invalid bearer token.
- `404 Not Found`: bookmark ID does not exist.
- `500 Internal Server Error`: unexpected store/server failure.

Repeated deletes should return `404 Not Found` after the first successful delete.
That is useful for this private tool because it catches mistaken IDs.

## Domain Changes

Add domain errors in `internal/bookmarks/bookmark.go`:

```go
var (
    ErrNotFound       = errors.New("bookmark not found")
    ErrDuplicateURL   = errors.New("bookmark url already exists")
    ErrNoUpdateFields = errors.New("no update fields")
)
```

Add an update input type:

```go
type UpdateInput struct {
    URL    *string `json:"url,omitempty"`
    Title  *string `json:"title,omitempty"`
    Notes  *string `json:"notes,omitempty"`
    Source *string `json:"source,omitempty"`
}
```

Use pointer fields so the code can distinguish omitted fields from fields that
are intentionally set to an empty string.

Examples:

- `{}` means no update fields and should fail.
- `{"title": ""}` means clear the title.
- `{"notes": ""}` means clear the notes.

For the first CRUD pass, omit tags from `UpdateInput` until tag storage is
implemented. Once tags are wired through the store, add `Tags *[]string` so the
code can distinguish omitted tags from intentionally clearing all tags.

Extend the store interface:

```go
type Store interface {
    CreateBookmark(ctx context.Context, input CreateInput) (Bookmark, bool, error)
    ListBookmarks(ctx context.Context) ([]Bookmark, error)
    UpdateBookmark(ctx context.Context, id string, input UpdateInput) (Bookmark, error)
    DeleteBookmark(ctx context.Context, id string) error
}
```

## SQLite Store Plan

The current schema already has the fields needed for edit and delete. No table
change is required for the first pass.

### Update SQL

The store should build a small dynamic `UPDATE` statement based on the fields
that are present.

For each present field:

- `URL`: trim original URL, normalize it, update both `url` and `normalized_url`.
- `Title`: update `title`.
- `Notes`: update `notes`.
- `Source`: update `source`.

Always update `updated_at` when the update succeeds.

Suggested flow:

1. Reject empty `id`.
2. Reject `UpdateInput` with no fields.
3. If `URL` is present, normalize it with `NormalizeURL`.
4. If normalized URL is present, check whether another bookmark already has it:

   ```sql
   SELECT id
   FROM bookmarks
   WHERE normalized_url = ? AND id <> ?
   LIMIT 1;
   ```

   If found, return `ErrDuplicateURL`.

5. Run the update:

   ```sql
   UPDATE bookmarks
   SET title = ?, notes = ?, source = ?, updated_at = ?
   WHERE id = ?;
   ```

   The exact columns should depend on present fields.

6. Check `RowsAffected`.
   - `0`: return `ErrNotFound`.
   - `1`: load and return the bookmark by ID.

Add a helper:

```go
func (s *SQLStore) bookmarkByID(ctx context.Context, id string) (Bookmark, error)
```

It should map `sql.ErrNoRows` to `ErrNotFound`.

### Delete SQL

```sql
DELETE FROM bookmarks
WHERE id = ?;
```

Then check `RowsAffected`:

- `0`: return `ErrNotFound`.
- `1`: return nil.

The existing `bookmark_tags` table has `ON DELETE CASCADE`, so hard delete will
also remove tag join rows when tags are eventually wired through.

## Store Tests

Add these to the existing store contract tests so every future store
implementation must behave the same way.

### Update Tests

- `UpdateBookmark` updates title, notes, and source.
- `UpdateBookmark` can clear title and notes with empty strings.
- `UpdateBookmark` updates URL and normalized URL.
- `UpdateBookmark` updates `updated_at` and preserves `created_at`.
- `UpdateBookmark` returns `ErrNotFound` for an unknown ID.
- `UpdateBookmark` returns `ErrNoUpdateFields` for an empty update input.
- `UpdateBookmark` returns existing URL validation errors when URL is invalid:
  - `ErrEmptyURL`
  - `ErrUnsupported`
  - `ErrMissingHost`
  - `ErrURLUserInfo`
- `UpdateBookmark` returns `ErrDuplicateURL` when the new normalized URL belongs
  to another bookmark.

### Delete Tests

- `DeleteBookmark` removes an existing bookmark.
- Deleted bookmark no longer appears in `ListBookmarks`.
- `DeleteBookmark` returns `ErrNotFound` for an unknown ID.
- Deleting one bookmark does not delete others.

## Server Plan

Add routes in `internal/server/server.go`:

```go
mux.HandleFunc("PATCH /api/bookmarks/{id}", s.requireAuth(s.handleUpdateBookmark))
mux.HandleFunc("DELETE /api/bookmarks/{id}", s.requireAuth(s.handleDeleteBookmark))
```

Use Go's `http.ServeMux` path variables:

```go
id := r.PathValue("id")
```

Add response type:

```go
type updateBookmarkResponse struct {
    Bookmark bookmarks.Bookmark `json:"bookmark"`
}
```

Reuse the JSON helpers already in the server package.

Suggested request body limit:

```go
const maxUpdateBookmarkBodyBytes = 64 * 1024
```

It can start equal to `maxCreateBookmarkBodyBytes`.

### Server Error Mapping

For update:

- URL validation errors and `ErrNoUpdateFields`: `400`.
- `ErrNotFound`: `404`.
- `ErrDuplicateURL`: `409`.
- unexpected errors: `500`.

For delete:

- `ErrNotFound`: `404`.
- unexpected errors: `500`.

## Server Tests

Add tests using the existing `fakeStore` style.

### Update Handler Tests

- `PATCH /api/bookmarks/{id}` requires bearer token.
- Valid patch calls `UpdateBookmark` with the path ID and decoded input.
- Valid patch returns `200` and JSON `{ "bookmark": ... }`.
- Empty update body maps to `400`.
- Bad JSON maps to `400`.
- Trailing JSON maps to `400`.
- Oversized body maps to `413`.
- URL validation errors map to `400`.
- `ErrNoUpdateFields` maps to `400`.
- `ErrNotFound` maps to `404`.
- `ErrDuplicateURL` maps to `409`.
- Unexpected store error maps to `500`.

### Delete Handler Tests

- `DELETE /api/bookmarks/{id}` requires bearer token.
- Valid delete calls `DeleteBookmark` with the path ID.
- Valid delete returns `204` and no JSON body.
- `ErrNotFound` maps to `404`.
- Unexpected store error maps to `500`.

## API Client Plan

Extend `internal/apiclient.Client`:

```go
func (c *Client) UpdateBookmark(ctx context.Context, id string, input bookmarks.UpdateInput) (bookmarks.Bookmark, error)
func (c *Client) DeleteBookmark(ctx context.Context, id string) error
```

Implementation should mirror the existing client methods:

- Build URLs as `c.baseURL + "/api/bookmarks/" + url.PathEscape(id)`.
- Use `http.NewRequestWithContext`.
- Set `Authorization: Bearer <token>`.
- For update, set `Content-Type: application/json`.
- Treat non-2xx statuses as errors.
- Decode update response JSON into `{ Bookmark bookmarks.Bookmark }`.
- Delete expects `204 No Content` and does not decode a body.

## API Client Tests

Use `httptest.Server`, following existing `internal/apiclient` tests.

### Update Client Tests

- Sends `PATCH /api/bookmarks/{id}`.
- Sends bearer token.
- Sends JSON body.
- Decodes returned bookmark.
- Returns error on non-2xx status.
- Path-escapes IDs.

### Delete Client Tests

- Sends `DELETE /api/bookmarks/{id}`.
- Sends bearer token.
- Returns nil on `204`.
- Returns error on non-2xx status.
- Path-escapes IDs.

## CLI UX Plan

Add commands:

```sh
bookmarkctl edit <id> [-url URL] [-title TITLE] [-notes NOTES] [-source SOURCE]
bookmarkctl delete <id>
bookmarkctl list -ids
```

### `edit`

Examples:

```sh
bookmarkctl edit abc123 -title "Better title"
bookmarkctl edit abc123 -notes ""
bookmarkctl edit abc123 -url https://example.com/new
```

Important detail: the CLI must distinguish an omitted flag from a flag set to an
empty value. The standard `flag` package does not directly expose whether a flag
was set, so add a tiny custom flag value type:

```go
type optionalStringFlag struct {
    value string
    set   bool
}
```

Its `Set` method should set both `value` and `set`.

Then `runEdit` can build `bookmarks.UpdateInput` with pointer fields only for
flags that were actually provided.

If no edit flags are set, return an error before loading config or creating the
client.

Output:

```text
updated <id> <url>
```

### `delete`

Examples:

```sh
bookmarkctl delete abc123
```

Behavior:

- Require exactly one ID.
- Call `DeleteBookmark`.
- Print:

  ```text
  deleted <id>
  ```

Do not prompt for confirmation in the first pass. This keeps scripts simple.
Confirmation can be added later behind an interactive flag if needed.

### `list -ids`

Examples:

```sh
bookmarkctl list -ids
```

Output:

```text
<id>\t<url>\t<title>
```

This is the bridge that makes edit/delete usable from the terminal.

## CLI Tests

Extend the local `bookmarkClient` interface in `cmd/bookmarkctl`:

```go
type bookmarkClient interface {
    CreateBookmark(context.Context, bookmarks.CreateInput) (bookmarks.Bookmark, bool, error)
    ListBookmarks(context.Context) ([]bookmarks.Bookmark, error)
    UpdateBookmark(context.Context, string, bookmarks.UpdateInput) (bookmarks.Bookmark, error)
    DeleteBookmark(context.Context, string) error
}
```

Extend the fake client with update/delete fields.

### Edit Tests

- `bookmarkctl edit <id> -title New` calls `UpdateBookmark` with `Title` set.
- `bookmarkctl edit <id> -title ""` calls `UpdateBookmark` with `Title` set to an empty string.
- `bookmarkctl edit <id> -notes ""` can clear notes.
- `bookmarkctl edit <id> -url https://example.com/new` sets URL.
- `bookmarkctl edit` requires an ID.
- `bookmarkctl edit <id>` with no flags returns an error and does not create a client.
- Client construction errors are returned.
- Client update errors are returned.
- Successful update prints `updated <id> <url>`.

### Delete Tests

- `bookmarkctl delete <id>` calls `DeleteBookmark`.
- `bookmarkctl delete` requires an ID.
- `bookmarkctl delete <id> extra` rejects extra args.
- Client construction errors are returned.
- Client delete errors are returned.
- Successful delete prints `deleted <id>`.

### List ID Tests

- `bookmarkctl list -ids` prints `id`, `url`, and `title` columns.
- Existing `bookmarkctl list` behavior still prints `url` and `title` columns.

## Migration Considerations

No schema migration is required for the first edit/delete pass because the
current `bookmarks` table already includes:

- `url`
- `normalized_url`
- `title`
- `notes`
- `source`
- `created_at`
- `updated_at`

Hard delete uses the existing primary key and foreign-key cascade behavior.

Before adding future schema changes, introduce a small migration mechanism:

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
) STRICT;
```

For this CRUD workstream, avoid adding migration machinery unless an actual
schema change becomes necessary.

## Ticket Breakdown

### Ticket 1: Store Contract For Update/Delete

Add domain types/errors and failing store contract tests.

Acceptance criteria:

- Tests describe update, delete, not found, empty update, invalid URL, and duplicate URL behavior.
- SQL store does not need to pass yet if doing strict red-green development.
- No server/client/CLI changes in this ticket.

### Ticket 2: SQL Store Update/Delete

Implement `UpdateBookmark`, `DeleteBookmark`, and `bookmarkByID`.

Acceptance criteria:

- Store contract tests pass.
- `updated_at` changes on successful update.
- `created_at` remains stable on update.
- Deleting removes the bookmark from list results.
- Duplicate normalized URL updates return `ErrDuplicateURL`.

### Ticket 3: Server Routes

Add `PATCH /api/bookmarks/{id}` and `DELETE /api/bookmarks/{id}`.

Acceptance criteria:

- Server tests cover auth, success, bad request, not found, conflict, oversized body, and internal error cases.
- Update returns JSON with the updated bookmark.
- Delete returns `204 No Content`.
- Existing create/list tests continue to pass.

### Ticket 4: API Client Methods

Add `UpdateBookmark` and `DeleteBookmark` to `internal/apiclient`.

Acceptance criteria:

- Client tests verify method, path, auth header, request body, response decoding, and non-2xx errors.
- IDs are path-escaped.
- Existing client tests continue to pass.

### Ticket 5: CLI List IDs

Add `bookmarkctl list -ids`.

Acceptance criteria:

- Default `list` output remains two columns: URL and title.
- `list -ids` prints three columns: ID, URL, title.
- Tests parse tab-separated output rather than comparing a large raw string.

### Ticket 6: CLI Edit

Add `bookmarkctl edit`.

Acceptance criteria:

- Supports `-url`, `-title`, `-notes`, and `-source`.
- Empty string flags intentionally clear fields.
- Omitted flags are not sent.
- Rejects missing ID, extra args, and no-op edits before creating a client.
- Success prints `updated <id> <url>`.

### Ticket 7: CLI Delete

Add `bookmarkctl delete`.

Acceptance criteria:

- Requires exactly one ID.
- Calls API client delete.
- Success prints `deleted <id>`.
- Not found and other API errors are returned clearly.

### Ticket 8: End-To-End Smoke Test

Manually verify the deployed or local server.

Acceptance criteria:

```sh
bookmarkctl add https://example.com/crud-smoke
bookmarkctl list -ids
bookmarkctl edit <id> -title "CRUD smoke"
bookmarkctl list -ids
bookmarkctl delete <id>
bookmarkctl list -ids
```

The bookmark should be created, edited, visible with the updated title, then
removed from list output after delete.
