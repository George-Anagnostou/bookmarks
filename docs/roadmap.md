# Roadmap

## Done

- CRUD API and `bookmarkctl` commands
- List search and pagination
- `bookmarkctl list` output formats (table, tsv, json)
- Deploy, rollback, and daily VPS backups
- iOS Shortcut for saving URLs (manual setup)

## Next

Priorities reflect current goals: mobile retrieval and desktop hotkey integration.

### CLI UX

- Hammerspoon or hotkey wrapper around `bookmarkctl`

### Mobile reading

- iPhone Shortcut for search-and-open (calls list API)
- RSS/Atom feed for recent bookmarks in a feed reader
- Optional HTML index if feeds and shortcuts are not enough

### Operations

- Offsite backup copy to a trusted machine (`rsync` pull)
- Restore drill documented as a recurring habit
- Optional deploy-user instead of root SSH

### Later

- Per-device revocable tokens (replace single shared bearer token)
- Tags support in store and API
- Title fetching for bookmarks saved without titles

## Constraints

- Go standard library where practical
- Private by default; no third-party services required
- Keep capture and retrieval friction low
- Prefer simple scripts over heavy automation

