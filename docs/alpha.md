# Bookmark Manager Alpha

This alpha should optimize for one thing: saving a URL from any device with almost no thought.

## Stack

- Go 1.26.4.
- Standard library for the application, plus `modernc.org/sqlite` as the sole direct dependency.
- One private server behind nginx.
- SQLite storage via `database/sql`.
- A small `Store` interface so HTTP handlers do not care about the backing store.

`modernc.org/sqlite` is a pure-Go SQLite driver. It brings transitive dependencies, but it avoids CGO and keeps deployment simple.

## Alpha Behavior

### Save Bookmark

`POST /api/bookmarks`

Headers:

```text
Authorization: Bearer <client-token>
Content-Type: application/json
```

Body:

```json
{
  "url": "https://example.com/article",
  "title": "Optional title",
  "tags": ["optional"],
  "notes": "Optional note",
  "source": "ios-share-sheet"
}
```

Rules:

- Accept only `http` and `https`.
- Accept bare domains by defaulting them to `https://`.
- Normalize scheme and host to lowercase.
- Remove default ports.
- Preserve path, query string, and fragment.
- Reject URLs containing credentials.
- Treat `normalized_url` as unique.
- If a bookmark already exists, return the existing bookmark with `created: false`.

Expected response:

```json
{
  "bookmark": {
    "id": "01J...",
    "url": "https://example.com/article",
    "normalized_url": "https://example.com/article",
    "title": "Optional title",
    "created_at": "2026-06-10T12:00:00Z",
    "updated_at": "2026-06-10T12:00:00Z"
  },
  "created": true
}
```

### List Bookmarks

`GET /`

For alpha, server-rendered HTML is enough:

- Newest bookmarks first.
- Title if present, otherwise URL.
- Domain visible.
- Search can wait until after create/list works.

`GET /api/bookmarks`

Return JSON for scripts and later clients.

## Storage Plan

Start with `data/bookmarks.db`.

```sql
CREATE TABLE IF NOT EXISTS bookmarks (
  id TEXT PRIMARY KEY,
  url TEXT NOT NULL,
  normalized_url TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT,
  read_at TEXT
) STRICT;

CREATE TABLE IF NOT EXISTS tags (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
) STRICT;

CREATE TABLE IF NOT EXISTS bookmark_tags (
  bookmark_id TEXT NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (bookmark_id, tag_id)
) STRICT;

CREATE INDEX IF NOT EXISTS bookmarks_created_at_idx
ON bookmarks(created_at DESC);
```
