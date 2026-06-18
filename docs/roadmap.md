# Bookmark Manager Roadmap

The alpha is now usable end to end:

- `bookmarkd` runs on the VPS behind nginx/systemd.
- `bookmarkctl add` can save bookmarks through the API.
- `bookmarkctl list` can retrieve bookmarks through the API.
- Direct `curl` requests work.
- The iPhone Shortcut can save shared URLs.

This document tracks the next buildouts. Each workstream should stay small enough to implement and review independently.

## Workstreams

1. Deployment automation and hardening
   - Document the current Debian/nginx/systemd setup.
   - Make updates repeatable with as little manual VPS work as possible.
   - Harden service permissions and deployment procedures.

2. Edit and delete
   - Turn the bookmark API into basic CRUD.
   - Add hard delete first.
   - Add metadata editing for title, notes, URL, and later tags.

3. Reading
   - Improve bookmark retrieval as the list grows.
   - Make reading usable from mobile.
   - Explore HTML, search, pagination, Shortcut retrieval, and lightweight app-like views.

4. Per-device tokens and auth
   - Move beyond one shared bearer token.
   - Add revocable device tokens.
   - Explore future sharing without committing to multi-user complexity too early.

5. Backups
   - Add a simple SQLite backup strategy on the VPS.
   - Include restore instructions and backup verification.

## Suggested Order

1. Backups
2. Deployment automation and hardening
3. Reading
4. Edit and delete
5. Per-device tokens and auth

Backups should come first because the system is now receiving real bookmarks. Deployment automation should follow because it reduces friction for every later release. Reading should come before deeper CRUD if mobile retrieval quickly becomes painful.

## Project Constraints

- Prefer Go standard library.
- Avoid third-party services where practical.
- Keep the server private by default.
- Keep mobile capture friction as close to zero as possible.
- Favor simple, inspectable operational procedures over clever automation.
