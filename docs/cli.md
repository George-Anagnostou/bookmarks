# CLI

`bookmarkctl` uses `BOOKMARKS_URL` and `BOOKMARKS_TOKEN` (environment or `.env.bookmarkctl`).

## Commands

```sh
bookmarkctl add [-title TITLE] [-notes NOTES] URL
bookmarkctl list [-l] [-query TERM] [-limit N] [-offset N] [-format FORMAT]
bookmarkctl edit [-url URL] [-title TITLE] [-notes NOTES] [-source SOURCE] ID
bookmarkctl delete <id>
```

| Command | Output |
|---------|--------|
| `add` | `added <url>` or `exists <url>` |
| `list` | See below |
| `edit` | `updated <id>` |
| `delete` | `deleted <id>` |

`add` sets `source` to `bookmarkctl`. `edit` requires at least one flag; an empty flag value clears that field.

## list output

The default format is a table with `ID`, `Title`, and `URL` columns.
Override it with `-format table`, `tsv`, or `json`.

| Format | Use |
|--------|-----|
| `table` | Human-readable columns (normal: ID + Title + URL; with `-l`: all fields) |
| `tsv` | Scripts and pipes: full bookmark fields (tab-separated, no header); `-l` has no effect |
| `json` | Full bookmark objects as a JSON array; `-l` has no effect |

Examples:

```sh
bookmarkctl list
bookmarkctl list -l
bookmarkctl list -query sqlite -limit 25
bookmarkctl list -format json | jq '.[].url'
bookmarkctl list | cut -f2
bookmarkctl list -l -format table
```

`-l` (long) only applies to the `table` output and shows all fields. It is rejected when used with `-format tsv` or `-format json`.

`-limit` 0 means no limit. Negative `-limit` or `-offset` is rejected before the API is called.
