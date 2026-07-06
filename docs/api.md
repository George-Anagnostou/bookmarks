# API

All `/api/*` routes require bearer auth:

```text
Authorization: Bearer <token>
```

`GET /healthz` is public.

## Create bookmark

```http
POST /api/bookmarks
Content-Type: application/json
```

Body:

```json
{
  "url": "https://example.com/article",
  "title": "Optional title",
  "notes": "Optional note",
  "source": "ios-shortcut"
}
```

Response `200`:

```json
{
  "bookmark": { "id": "...", "url": "...", "normalized_url": "...", "title": "...", "created_at": "...", "updated_at": "..." },
  "created": true
}
```

If the normalized URL already exists, returns the existing bookmark with
`"created": false`.

Validation errors return `400`. Duplicate URL on update returns `409`.

## List bookmarks

```http
GET /api/bookmarks
GET /api/bookmarks?query=sqlite&limit=25&offset=0
```

Query parameters:

| Param | Default | Notes |
|-------|---------|-------|
| `query` | empty | Case-insensitive search across url, title, notes, source |
| `limit` | 0 (no limit) | Must be non-negative |
| `offset` | 0 | Must be non-negative |

Results are newest first.

Response `200`:

```json
{
  "bookmarks": [
    {
      "id": "...",
      "url": "https://example.com/article",
      "normalized_url": "https://example.com/article",
      "title": "Example",
      "notes": "",
      "source": "bookmarkctl",
      "created_at": "2026-06-18T12:00:00Z",
      "updated_at": "2026-06-18T12:00:00Z"
    }
  ]
}
```

Invalid `limit` or `offset` returns `400`.

## Update bookmark

```http
PATCH /api/bookmarks/{id}
Content-Type: application/json
```

Send only fields to change:

```json
{
  "title": "New title",
  "notes": "",
  "url": "https://example.com/new"
}
```

Response `200` returns the updated bookmark. `404` if id not found.

## Delete bookmark

```http
DELETE /api/bookmarks/{id}
```

Response `204` on success. `404` if id not found.

## URL rules

- `http` and `https` only
- Bare domains default to `https://`
- Scheme and host normalized to lowercase; default ports removed
- Credentials in URLs rejected
- `normalized_url` is unique