# Bookmarks

A private bookmark manager for saving URLs from phones, laptops, and scripts.

The alpha target is a small Go web server behind nginx:

- `POST /api/bookmarks` saves URLs from iOS Shortcuts, Android, shells, and browser tooling.
- `GET /` lists saved bookmarks in a private server-rendered page.
- SQLite is the primary store.
- [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) is the only direct third-party dependency.

See [docs/alpha.md](docs/alpha.md) for the endpoint contract, schema, and implementation order.

## Development

```sh
go test ./...
```
