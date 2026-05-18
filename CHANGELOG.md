# Changelog

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
