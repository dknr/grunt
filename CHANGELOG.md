# Changelog

## v0.5.7 — Profile Pictures, E2E Tests, Date Dividers & Template Formatting

### Features
- Profile picture upload with image processing, ETag caching, and tests (avatar_test.go)
- Playwright E2E test suite covering auth, chat, settings, SSE streaming, and mobile viewport (33 tests)
- Sticky date dividers ("Today", "Yesterday", "Monday, January 2") between messages in the chat stream
- Invite code registration made atomic via single SQL transaction with concurrent race-condition protection

### Fixes
- Fixed profile picture cache control headers for proper browser caching
- Reformatted SSE message template across multiple lines: user content newlines converted to `<br>` before rendering, multi-line HTML sent as separate SSE `data:` fields per spec

### Tests
- 6 Playwright E2E test files (auth, avatar, chat, mobile, settings, SSE)
- Avatar processing tests (PNG, JPEG, square, wide, too-large, invalid format, empty input)
- Invite transaction atomicity tests with concurrent race conditions (10 goroutines)

### Refactoring
- Unified `Sync` query ordering: subquery with `ORDER BY DESC LIMIT` wrapped in outer `ORDER BY ASC`, removing fragile manual slice reversal

## v0.5.6 — Auth Fixes, Hub Lifecycle, & Message Limits

### Fixes
- Eliminated TokenStore data race: expired session tokens are now deleted under a write lock instead of a read lock
- Fixed ExtractToken: non-Bearer Authorization header values are no longer mistakenly accepted as tokens
- Added `used_by_user` column to invites table, preserving the original `created_by_user` audit trail (previously overwritten by the registering user)
- Added defensive error when `MarkInviteUsed` affects zero rows, catching double-use or nonexistent invite codes early
- Added `Hub.Stop()` method for graceful goroutine termination, wired into server shutdown
- Aligned cold-start invite code expiry from 24h to 10m to match admin-generated codes

### Features
- Added 10KB message content length limit with early Content-Length header rejection (413 Payload Too Large) and post-parse content check

### Refactoring
- Unified `Sync` query ordering: subquery with `ORDER BY DESC LIMIT` wrapped in outer `ORDER BY ASC`, removing fragile manual slice reversal

### Tests
- Added 21 auth unit tests covering ExtractToken, ValidateToken (session, API key, expiry, concurrency), and context helpers
- Added 8 invite unit tests covering create, validate, expiry, single-use, double-use rejection, audit trail preservation, and multi-invite independence
- Fixed `TestCreateUser` to expect duplicate insert error instead of silently succeeding
- Strengthened `TestSyncMessages` with ascending sort order verification and edge cases

## v0.5.5 — Emote System & Runtime File Watcher

### Features
- Server-side emote rendering with `:name:` token replacement and `<img>` tag generation
- Static emote directory (`server/static/emotes/`) scanned at startup for `.svg`, `.png`, `.gif`, `.webp` files
- Build-in emoji map with built-in tokens (:smile:, :heart:) and image-based emotes
- Runtime emote file watcher using `fsnotify` for live reload of image emotes without restart
- Emote directory resolved via `$GRUNT_EMOTE_DIR` > `$XDG_DATA_HOME/grunt/emotes` > `$HOME/.local/share/grunt/emotes`
- Serve runtime emotes at `/emotes/<filename>` from disk with proper Content-Type headers
- Thread-safe `emoteMap` with `sync.RWMutex` for concurrent `ReplaceEmotes` calls
- Startup log shows resolved path and emote count; file add/remove events are logged

### Fixes
- Updated message template to use `{{.RenderedContent}}` instead of `{{.Content}}` for emote rendering

## v0.5.4 — Admin Settings & Version Improvements

### Features
- Added admin user and API key management to settings page

### Fixes
- Improved version string calculation to show commits ahead of the last tag

## v0.5.3 — Emoji Fonts & Documentation Update

### Features
- Integrated Google Fonts CDN for Noto Color Emoji rendering
- Appended `'Noto Color Emoji'` to `--font-sans` CSS variable as fallback

### Documentation
- Rewrote README with comprehensive architecture, API reference, and testing sections

## v0.5.2 — Web UI Redesign & Mobile Support

### Architecture
- Migrated to server-side rendering with `html/template` + HTMX (replacing client-side JS)
- Unified auth middleware across all endpoints
- Consolidated login/register into single handler

### Features
- Password change in settings page
- Logout button on settings page
- First-user admin grant on registration
- Build version footer (version, commit, timestamp) on login/settings pages
- Admin panel with invite code generation via HTMX

### UI
- Complete dark theme redesign
- Hash-based avatars for users
- Grouped message layout with username bubbles and timestamps
- Settings page with password change, logout, admin actions

### Mobile
- `interactive-widget=resizes-content` viewport meta tag for Chrome Android keyboard handling
- Dynamic viewport units (`dvw`/`dvh`) for proper layout during keyboard transitions
- Safe area padding for notched devices
- Fixed send button sizing (40x40px circle)
- Input clearing after send without losing focus

### Known Issues
- Version footer shows `Grunt ()` on invalid login/register (error handlers don't pass version data — cosmetic, to be fixed in v0.5.3)
- Keyboard closes after form submit (Enter key on virtual keyboard still works — deferred for later iteration)

## v0.5.1 — Web UI Registration Form

### Features
- Added registration form to web UI using existing invite code system

## v0.5.0 — Admin System, API Keys & HTMX Chat UI

### Features
- Admin system with API key authentication (`gk_` prefixed keys)
- HTMX-based chat web UI
- Token authentication via query parameter support
- Deno send client integrated into test orchestration

### Cleanup
- Removed unused gorilla/websocket dependency
- Updated README and added CHANGELOG

## v0.4.1 — Bugfixes

### Fixes
- Corrected igor system prompts to match actual message format
- Fixed SSE `data:` prefix stripping (removed trailing space)

## v0.4.0 — SSE Migration & REST API

### Breaking Changes
- Replaced WebSocket with Server-Sent Events (SSE) for pub/sub messaging
- Clients must accept `text/event-stream` for streaming endpoints

### Features
- REST POST `/api/chat/message` endpoint for sending messages
- Split-pane client layout for recv and repl windows

## v0.3.0 — Invite Codes & Test Environment

### Features
- Invite code system with 10-minute expiration
- Automated test environment orchestrator (tmux-based setup with server, clients, and igor instances)

### Fixes
- Eliminated data race in integration tests

## v0.2.0 — Auth Middleware & API Foundation

### Features
- Auth middleware for request validation (Bearer token from header)
- OpenAPI codegen for server interfaces
- API endpoints organized under `/api/` prefix
- Server and storage package reorganization
- Client library restructuring with auth header support

## v0.1.0 — Initial Release

### Features
- Client library restructuring
- Initial web UI
