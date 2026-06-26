# Reading And Mobile Retrieval

The capture path now works: `bookmarkctl`, curl, and the iPhone Shortcut can save bookmarks through the API. The next reading problem is different: as the list grows, terminal output becomes awkward, and mobile users need a fast way to find and open saved links.

This document explores reading options and proposes a phased build plan.

## Goals

- Retrieve bookmarks easily from a phone.
- Keep the first read experience private and simple.
- Preserve the API-first shape of the project.
- Avoid third-party dependencies unless they clearly remove real complexity.
- Keep the web surface small enough to audit.
- Make search useful before the database grows too large.

## Current Baseline

Implemented today:

- `POST /api/bookmarks` saves bookmarks.
- `GET /api/bookmarks` returns all bookmarks as JSON.
- `bookmarkctl add` creates bookmarks.
- `bookmarkctl list` prints tab-separated `id url title` rows.
- Auth is a single bearer token.

Current gaps:

- No browser/mobile reading UI.
- No search or filtering.
- No pagination.
- No endpoint for fetching one bookmark.
- No RSS/Atom feed.
- No static export.
- No optimized response shape for small mobile clients.

## Options

### RSS Or Atom Feed

Expose recent bookmarks as a private RSS or Atom feed.

The feed is a strong fit for this project because bookmarks are naturally a
personal stream of links. Feed readers already solve much of the mobile reading
problem: chronological browsing, opening links, sharing links, and read/unread
state.

Possible route:

```text
GET /feeds/recent.xml
```

Feed item mapping:

```text
title: bookmark title, or URL when title is blank
link: original bookmarked URL
description: notes, source, and saved date
guid: bookmark ID
pubDate: created_at
```

Advantages:

- Works well for newest-first mobile reading.
- Uses existing feed reader apps instead of requiring a custom frontend.
- Feed readers handle read/unread state, opening links, and sharing.
- XML generation is possible with the Go standard library.
- Avoids deciding on browser session auth before we know we need a web UI.

Disadvantages:

- Feed readers often do not support bearer auth cleanly.
- Search is not the main interaction model.
- A feed token in a URL is convenient but should be treated as a secret.
- Editing/deleting bookmarks is outside the feed workflow.

Best use:

- Primary mobile reading surface after API list/search/pagination is improved.

### HTML Index

A server-rendered HTML page at `GET /` or `GET /bookmarks`.

Advantages:

- Works immediately on iPhone, Android, laptops, and BSD/Linux browsers.
- No app install.
- No JavaScript required for the first version.
- Easy to keep private behind the existing auth model if we decide how browser auth should work.
- Go's `html/template` is enough.

Disadvantages:

- Bearer auth is awkward in a normal browser.
- If we add cookie auth, the browser path becomes a separate auth path from the API.
- Search and pagination need server-side form handling.

Best use:

- Later browser-based reading and management if RSS is not enough.
- Read-only browser UI with search, pagination, and open/copy links.

### JSON Endpoint Improvements

Extend `GET /api/bookmarks` with query params:

```text
GET /api/bookmarks?q=sqlite&limit=50&offset=0
GET /api/bookmarks?tag=go&limit=50
```

Advantages:

- Keeps all client types on the same API.
- Makes CLI, Shortcut, and future HTML UI easier to build.
- Works with the current bearer-token auth.
- Good foundation for pagination and search.

Disadvantages:

- Not a complete mobile reading solution by itself.
- Raw JSON is not pleasant in a phone browser.

Best use:

- The next backend feature before a richer UI.

### Search

Start with simple SQL matching across URL, title, and notes:

```sql
WHERE normalized_url LIKE ? OR title LIKE ? OR notes LIKE ?
```

Later, if needed, move to SQLite FTS.

Advantages:

- High value once the bookmark count grows.
- Simple SQL search is enough for early use.
- No new dependency.

Disadvantages:

- `LIKE` search is basic.
- Ranking will be weak.
- Large databases may eventually need FTS indexes.

Best use:

- Early reading feature, paired with pagination.

### Pagination

Add `limit` and `offset`, or use cursor pagination based on `created_at` and `id`.

For this project, start with:

```text
limit: default 50, max 100
offset: default 0
```

Advantages:

- Simple to implement and test.
- Works well enough for a single-user SQLite app.
- Easy for CLI and HTML forms.

Disadvantages:

- Offset pagination can shift if new bookmarks are added while paging.
- Cursor pagination is more stable, but more code.

Best use:

- First pagination implementation.

### iPhone Shortcut Retrieval UI

Create a second Shortcut named something like "Open Bookmark" or "Search Bookmarks".

Possible flow:

1. Ask for search text.
2. Call `GET /api/bookmarks?q=<text>&limit=20`.
3. Extract titles or URLs.
4. Show "Choose from List".
5. Open selected URL.

Advantages:

- Stays close to the current mobile workflow.
- Avoids browser auth for now.
- Can use the same bearer token header as the save Shortcut.
- Fast to build once API search exists.

Disadvantages:

- Shortcut UI is limited.
- Harder to browse visually.
- Editing complex metadata from Shortcuts will be clumsy.

Best use:

- Fastest mobile retrieval path after API search lands.

### Simple PWA

A minimal installable web app with a manifest, responsive HTML, and maybe a small amount of JavaScript.

Advantages:

- Feels app-like on iPhone.
- Can expose search and tap-to-open workflows.
- Could cache a small recent list.

Disadvantages:

- Adds frontend complexity.
- Auth becomes more important.
- Offline behavior can become a distraction.

Best use:

- Later, after the plain HTML page proves the interaction model.

### JSON Feed

Expose recent bookmarks using the JSON Feed format.

Advantages:

- Easier to generate and test than XML.
- Maps naturally to the existing JSON API.
- Some modern feed readers support it.

Disadvantages:

- RSS/Atom has broader reader support.
- Does not solve the feed authentication problem.

Best use:

- Optional companion to RSS/Atom after the XML feed works.

### Browser Bookmarklet

A small JavaScript bookmarklet for desktop browsers that either saves or opens the bookmark search page.

Advantages:

- Simple desktop integration.
- Good fallback where browser extensions are too much.

Disadvantages:

- Mobile browser support is awkward.
- Bearer token handling from a bookmarklet is not ideal.
- Less useful now that saving already works from CLI and iOS Shortcut.

Best use:

- Later desktop convenience feature.

### Static Export

Generate a static HTML file or JSON export of bookmarks.

Advantages:

- Great backup and portability story.
- Can publish a private snapshot somewhere else if needed.
- No runtime auth complexity for local copies.

Disadvantages:

- Not ideal for live mobile retrieval.
- Exported files can leak private links if mishandled.

Best use:

- Backup/export feature, not the primary reading UI.

## Recommendation

Build reading in this order:

1. Improve the JSON list endpoint with search and pagination.
2. Add `bookmarkctl list` flags that use those API features.
3. Add a private RSS or Atom feed for recent bookmarks.
4. Decide on a read-only feed token strategy.
5. Build an iPhone "Search Bookmarks" Shortcut that calls the improved JSON endpoint and opens the selected URL.
6. Add a read-only HTML index only if feed-based reading is not enough.
7. Revisit auth for browser reading before exposing a browser UI.

This keeps the core API strong, uses feed readers for the main mobile reading workflow, and avoids forcing a browser-auth decision too early.

## Proposed API Shape

Extend the existing endpoint:

```text
GET /api/bookmarks
GET /api/bookmarks?q=sqlite
GET /api/bookmarks?limit=25&offset=50
GET /api/bookmarks?q=sqlite&limit=25&offset=0
```

Response:

```json
{
  "bookmarks": [
    {
      "id": "01...",
      "url": "https://example.com/article",
      "normalized_url": "https://example.com/article",
      "title": "Example",
      "notes": "",
      "source": "ios-shortcut",
      "created_at": "2026-06-18T12:00:00Z",
      "updated_at": "2026-06-18T12:00:00Z"
    }
  ],
  "limit": 25,
  "offset": 0,
  "count": 1
}
```

Notes:

- `limit` should default to `50`.
- `limit` should be capped at `100`.
- `offset` should default to `0`.
- Invalid `limit` or `offset` should return `400`.
- `q` should trim whitespace.
- Empty `q` should behave like no search query.
- Results should remain newest-first.

The response does not need a total count in the first version. A total count requires a second query and is not necessary for initial mobile retrieval.

## Proposed CLI Shape

Extend `bookmarkctl list`:

```sh
bookmarkctl list
bookmarkctl list -q sqlite
bookmarkctl list -limit 25
bookmarkctl list -offset 25
```

Output should remain tab-separated:

```text
https://example.com/article	Example title
```

Later additions:

```sh
bookmarkctl open -q sqlite
bookmarkctl open <id>
```

For now, `open` should wait. It is OS-specific and less important than search.

## Proposed Feed Shape

Start with one recent-bookmarks feed:

```text
GET /feeds/recent.xml
```

Use RSS 2.0 or Atom. RSS 2.0 is widely supported and simple enough for a first
implementation. Atom has cleaner IDs and timestamps. Either is acceptable; pick
one and keep the output boring and standards-compliant.

Initial feed behavior:

- Newest bookmarks first.
- Default limit: `50`.
- Maximum limit: `100`.
- Feed title: `Bookmarks`.
- Item title: bookmark title when present, otherwise URL.
- Item link: original bookmarked URL.
- Item GUID: bookmark ID.
- Item date: `created_at`.
- Item description: notes when present, plus source/saved metadata if useful.

Possible query params:

```text
GET /feeds/recent.xml?limit=50
GET /feeds/recent.xml?q=sqlite&limit=50
```

Searchable feeds are useful later, but the first feed should be recent-only.
Build search into the JSON API first, then decide whether search feeds are worth
the extra surface area.

### Feed Authentication

The current API bearer token is write-capable and should not be the long-term
feed credential. Feed readers often do not support bearer auth, so the practical
first design is a separate read-only feed token.

Possible forms:

```text
GET /feeds/recent.xml?token=<feed-token>
GET /feeds/<feed-token>/recent.xml
```

The path-token form is easier to paste into feed readers and avoids query-string
handling quirks, but both forms can leak through logs. Treat feed URLs as
secrets.

Recommended first version:

- Add `BOOKMARKS_FEED_TOKEN` as a separate server env var.
- Require the feed token for feed routes.
- Keep feed tokens read-only.
- Do not allow feed tokens to call write API routes.
- Document nginx/access-log implications before production use.

Later, per-device tokens can replace this with proper scoped tokens:

```text
scope: bookmarks:read
scope: bookmarks:write
scope: feeds:read
```

## Proposed Mobile Shortcut

Create a second iPhone Shortcut called "Search Bookmarks".

Flow:

1. Ask for input: "Search bookmarks".
2. URL encode the search text.
3. Get contents of `https://bookmarks.example.com/api/bookmarks?q=<query>&limit=20`.
4. Send `Authorization: Bearer <token>`.
5. Parse JSON.
6. Build list items from title when present, otherwise URL.
7. Choose from list.
8. Open selected bookmark URL.

If the Shortcut cannot preserve both display title and URL cleanly, the API can later add a compact Shortcut-specific endpoint:

```text
GET /api/bookmarks/choices?q=sqlite&limit=20
```

Response:

```json
[
  {
    "label": "Example title - example.com",
    "url": "https://example.com/article"
  }
]
```

Do not add this endpoint until the plain JSON response proves awkward in Shortcuts.

## Tickets

### Ticket 1: Add List Query Type To Domain

Add a query struct to `internal/bookmarks` for listing options.

Proposed type:

```go
type ListQuery struct {
    Query  string
    Limit  int
    Offset int
}
```

Acceptance criteria:

- Store interface supports listing with `ListQuery`.
- Existing callers are updated.
- Empty query returns normal newest-first results.
- Tests cover default query behavior.

### Ticket 2: Add SQL Search And Pagination

Update the SQLite store to support `q`, `limit`, and `offset`.

Acceptance criteria:

- Results remain newest-first.
- `q` searches URL, normalized URL, title, and notes.
- Search is case-insensitive enough for normal SQLite usage.
- `limit` and `offset` are applied.
- Tests cover:
  - default listing
  - title search
  - URL search
  - notes search
  - limit
  - offset
  - empty query

### Ticket 3: Add API Query Params

Update `GET /api/bookmarks` to parse query params and pass them to the store.

Acceptance criteria:

- `GET /api/bookmarks?q=sqlite&limit=25&offset=0` works.
- Missing params use defaults.
- Invalid numeric params return `400`.
- `limit` is capped at `100`.
- Response includes `bookmarks`, `limit`, `offset`, and `count`.
- Existing auth behavior is unchanged.
- Tests cover valid params, defaults, invalid params, and auth.

### Ticket 4: Update API Client

Extend `internal/apiclient` with list options.

Possible shape:

```go
type ListOptions struct {
    Query  string
    Limit  int
    Offset int
}

func (c *Client) ListBookmarks(ctx context.Context, opts ListOptions) ([]bookmarks.Bookmark, error)
```

Acceptance criteria:

- Client sends only non-zero query params.
- Existing CLI list behavior still works.
- Tests cover query encoding and response decoding.

### Ticket 5: Update CLI List Flags

Add `-q`, `-limit`, and `-offset` to `bookmarkctl list`.

Acceptance criteria:

- `bookmarkctl list` still works.
- `bookmarkctl list -q sqlite` passes the query to the API client.
- `bookmarkctl list -limit 25 -offset 25` passes pagination options.
- Output remains tab-separated.
- Tests use fake client assertions.

### Ticket 6: Add RSS Or Atom Feed

Add the first private feed for recent bookmarks.

Initial route:

```text
GET /feeds/recent.xml
```

Acceptance criteria:

- Feed contains recent bookmarks newest-first.
- Feed uses bookmark IDs as stable item identifiers.
- Feed item links point to the original bookmarked URLs.
- Feed titles use bookmark title when present, otherwise URL.
- Feed output validates in at least one common feed reader.
- Tests cover status code, auth behavior, content type, and at least one item.

### Ticket 7: Add Read-Only Feed Token

Add a separate read-only token for feed retrieval.

Acceptance criteria:

- `BOOKMARKS_FEED_TOKEN` can be configured separately from `BOOKMARKS_TOKEN`.
- Feed routes require the feed token.
- Feed token cannot create, edit, or delete bookmarks.
- Missing or wrong feed token returns `401`.
- Documentation warns that feed URLs should be treated as secrets.

### Ticket 8: Build iPhone Search Shortcut

Create and document the mobile retrieval Shortcut.

Acceptance criteria:

- Shortcut asks for search text.
- Shortcut calls the API with bearer auth.
- Shortcut displays a selectable list of results.
- Shortcut opens the selected URL.
- `docs/shortcuts.md` includes setup steps and troubleshooting notes.

### Ticket 9: Add Read-Only HTML Index

Add a private browser page after API search is stable.

Initial route:

```text
GET /bookmarks?q=sqlite&limit=50&offset=0
```

Acceptance criteria:

- Page lists bookmark title, URL, domain, and created date.
- Page has a search form.
- Page has previous/next pagination links.
- Links open normally on mobile.
- HTML uses `html/template`.
- No JavaScript required for first version.
- Auth strategy is explicitly chosen before exposing the route.

### Ticket 10: Evaluate Browser Auth

Decide how browser reading should authenticate.

Options:

- Basic auth at nginx.
- Cookie login in `bookmarkd`.
- Continue API-only bearer token and skip browser UI.
- Per-device tokens plus browser session cookies.

Acceptance criteria:

- Decision is documented.
- Threat model is documented at a practical level.
- Chosen approach works on iPhone Safari without repeated login.

### Ticket 11: Optional JSON Feed

Add JSON Feed after RSS/Atom works if it helps a preferred reader or client.

Acceptance criteria:

- `GET /feeds/recent.json` returns recent bookmarks in JSON Feed format.
- JSON feed uses the same auth strategy as the XML feed.
- Documentation explains why both feed formats exist.

### Ticket 12: Optional Static Export

Add an export command or endpoint.

Acceptance criteria:

- Can export bookmarks to JSON.
- Optional HTML export can be opened locally.
- Export documentation warns that exported files contain private URLs.

## Open Questions

- Should mobile reading be optimized for search-first or newest-first browsing?
- Should the first feed be RSS 2.0 or Atom?
- Should the feed token be passed as a query param or as part of the path?
- Do we want browser auth now, or should feed-based reading come first?
- Should the API client keep a backward-compatible `ListBookmarks(ctx)` method and add a separate `SearchBookmarks`, or should it switch to `ListBookmarks(ctx, opts)`?
- Should title fetching be added before the HTML page, so mobile lists are easier to scan?
- How much metadata should the Shortcut display before opening a link?

## Suggested Next Build

Start with API search and pagination. It is the foundation for CLI improvements,
Shortcut retrieval, feeds, and any future HTML index.

The smallest useful implementation is:

1. Add `ListQuery`.
2. Update SQL listing.
3. Parse `q`, `limit`, and `offset` in `GET /api/bookmarks`.
4. Update the API client.
5. Add CLI flags.
6. Add the recent-bookmarks RSS or Atom feed.
7. Add a separate read-only feed token.
8. Build the iPhone search Shortcut manually and document it.
