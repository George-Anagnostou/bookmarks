# CLI

`bookmarkctl` talks to the API using `BOOKMARKS_URL` and `BOOKMARKS_TOKEN` from
the environment (or a local env file).

## Commands

### add

```sh
bookmarkctl add <url> [-title TITLE] [-notes NOTES]
```

Prints `created <url>` or `exists <url>`.

Sets `source` to `bookmarkctl`.

### list

```sh
bookmarkctl list
bookmarkctl list -query sqlite -limit 25 -offset 0
```

Prints tab-separated rows: `id`, `url`, `title`.

Flags:

| Flag | Default | Notes |
|------|---------|-------|
| `-query` | empty | Search term passed to the API |
| `-limit` | 0 | Page size; 0 means no limit |
| `-offset` | 0 | Skip N results |

Negative `-limit` or `-offset` is rejected before contacting the server.

### edit

```sh
bookmarkctl edit <id> [-url URL] [-title TITLE] [-notes NOTES] [-source SOURCE]
```

At least one flag is required. Omitted flags are left unchanged; a flag set to an
empty string clears that field.

Prints `updated <id> <url>`.

### delete

```sh
bookmarkctl delete <id>
```

Prints `deleted <id>`. No confirmation prompt.

## Configuration

Copy `.env.bookmarkctl.example` and set:

```text
BOOKMARKS_URL=https://bookmarks.example.com
BOOKMARKS_TOKEN=<token>
```