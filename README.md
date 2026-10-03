# Bookmarks

A private bookmark manager for saving and retrieving URLs from phones, laptops,
and scripts.

## What works today

- **`bookmarkd`** — Go API server behind nginx on a Debian VPS
- **`bookmarkctl`** — CLI for add, list, edit, and delete
- **SQLite storage** with URL normalization and duplicate detection
- **List search and pagination** via the API and CLI
- **Daily VPS backups** via systemd timer
- **Deploy and rollback** scripts from a dev machine

There is no browser UI yet. Reading is via the CLI, API, or future mobile
shortcuts and feeds.

## Stack

- Go standard library + [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)
- One bearer token for API auth
- nginx → `bookmarkd` → SQLite

## Quick start (local)

```sh
cp .env.bookmarkd.example .env
cp .env.bookmarkctl.example .env.bookmarkctl

# edit both files with a shared token and URL

go run ./cmd/bookmarkd
go run ./cmd/bookmarkctl add https://example.com -title "Example"
go run ./cmd/bookmarkctl list
```

## Development

```sh
make test          # run all tests
make check         # formatting, script syntax, tests, vet, and race tests
make build-cli     # build bookmarkctl into dist/
make install-cli   # install bookmarkctl locally
make build-server  # cross-compile bookmarkd for Linux
```

## Production

```sh
cp .env.deploy.example .env.deploy
# set BOOKMARKS_DOMAIN and BOOKMARKS_DEPLOY_HOST

make update          # check, build, install CLI, deploy server, verify /healthz
make verify          # verify the deployed server health endpoint
make rollback        # restore previous bookmarkd binary
make install-backups # install daily SQLite backups on the VPS
```

See [docs/operations.md](docs/operations.md) for VPS layout, backups, and restore.

## Documentation

| Doc | Contents |
|-----|----------|
| [docs/api.md](docs/api.md) | HTTP API reference |
| [docs/cli.md](docs/cli.md) | `bookmarkctl` commands |
| [docs/operations.md](docs/operations.md) | Deploy, backups, restore |
| [docs/roadmap.md](docs/roadmap.md) | Planned work |

## Project layout

```text
cmd/bookmarkd/        API server startup and dependency wiring
cmd/bookmarkctl/      CLI client
internal/bookmarks/  bookmark types, errors, and URL normalization
internal/access/     person/client types and token helpers
internal/sqlite/     SQLite connection, schema, and persistence operations
internal/server/     HTTP handlers and their BookmarkStore interface
internal/apiclient/  HTTP client used by CLI
internal/fetcher/    page title fetching
scripts/             bootstrap, deploy, backup install
```
