# Roadmap

## Done

- CRUD API and `bookmarkctl` commands
- List search and pagination
- `bookmarkctl list` output formats (table, tsv, json)
- Deploy, rollback, and daily VPS backups
- iOS Shortcut for saving URLs (manual setup)
- SSRF-safe title fetching for bookmarks saved without titles

## Next

### CLI UX

- Polish terminal appearance and output.
- Improve table sizing, truncation, timestamps, colors, and error messages.
- Add an `open` workflow for quickly opening a bookmark.
- Support clipboard and stdin capture improvements.
- Support multiple CLI configuration profiles.

### Mobile

- Explore mobile-friendly retrieval and interaction options.
- Keep the direction open while evaluating Shortcuts, a lightweight web UI, PWA
  behavior, and feeds.

### Operations

- Offsite backup copy to a trusted machine (`rsync` pull).
- Restore drill documented as a recurring habit.
- Optional deploy-user instead of root SSH.

## Later

### Device Access

- Replace the shared bearer token with per-device revocable tokens.
- Support device names, expiration, last-used tracking, and local CLI profiles.
- Consider one-time pairing or QR-based enrollment.
- Keep this low priority until the main usability work is complete.

### Ideas: Capture

- Browser extension or bookmarklet.
- Better mobile share-sheet support.
- Automatic source and device metadata.
- Browser bookmark import.
- Bulk import with duplicate reporting.
- Optional note prompt during capture.

### Ideas: Retrieval

- Tags and tag-based filtering.
- Archive and read/unread states.
- Search filters for tags, sources, and date ranges.
- Saved views such as recent, unread, and untagged.
- Improved result ordering.
- Bulk archive, tagging, and deletion.
- JSON, TSV, and portable database export.

## Constraints

- Go standard library where practical
- Private by default; no third-party services required
- Keep capture and retrieval friction low
- Prefer simple scripts over heavy automation
