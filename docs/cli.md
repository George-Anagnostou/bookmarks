# CLI

`bookmarkctl` uses `BOOKMARKS_URL` and `BOOKMARKS_TOKEN` (environment or `.env.bookmarkctl`).

## Commands

```sh
bookmarkctl add <url> [-title TITLE] [-notes NOTES]
bookmarkctl list [-l] [-query TERM] [-limit N] [-offset N] [-output FORMAT]
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
| `table` | Human-readable columns (normal: Title + URL; with `-l`: all fields) |
| `tsv` | Scripts and pipes: full bookmark fields (tab-separated, no header); `-l` has no effect |
| `json` | Full bookmark objects as a JSON array; `-l` has no effect |

Examples:

```sh
bookmarkctl list
bookmarkctl list -l
bookmarkctl list -query sqlite -limit 25
bookmarkctl list -output json | jq '.[].url'
bookmarkctl list | cut -f2
bookmarkctl list -l -output table
```

`-l` (long) only applies to the `table` output and shows all fields. It is rejected when used with `-output tsv` or `-output json`.

`-limit` 0 means no limit. Negative `-limit` or `-offset` is rejected before the API is called.