# CLI

`bookmarkctl` uses `BOOKMARKS_URL` and `BOOKMARKS_TOKEN` (environment or `.env.bookmarkctl`).

## Commands

```sh
bookmarkctl add <url> [-title TITLE] [-notes NOTES]
bookmarkctl list [-query TERM] [-limit N] [-offset N] [-output FORMAT]
bookmarkctl edit <id> [-url URL] [-title TITLE] [-notes NOTES] [-source SOURCE]
bookmarkctl delete <id>
```

| Command | Output |
|---------|--------|
| `add` | `created <url>` or `exists <url>` |
| `list` | See below |
| `edit` | `updated <id> <url>` |
| `delete` | `deleted <id>` |

`add` sets `source` to `bookmarkctl`. `edit` requires at least one flag; an empty flag value clears that field.

## list output

Default format depends on where stdout goes:

| stdout | default `-output` |
|--------|-------------------|
| terminal | `table` |
| pipe or file | `tsv` |

Override with `-output table`, `tsv`, or `json`.

| Format | Use |
|--------|-----|
| `table` | Human-readable columns: title, url, id |
| `tsv` | Scripts and pipes: `title`, `url`, `id` (tab-separated, no header) |
| `json` | Full bookmark objects as a JSON array |

Examples:

```sh
bookmarkctl list
bookmarkctl list -query sqlite -limit 25
bookmarkctl list -output json | jq '.[].url'
bookmarkctl list | cut -f2
```

`-limit` 0 means no limit. Negative `-limit` or `-offset` is rejected before the API is called.