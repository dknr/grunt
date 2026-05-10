# Changelog

## v0.4.1 — Bug Fixes & Deno Client
- **Fix:** Corrected igor system prompts — bots now recognize actual usernames instead of confusing the placeholder `user` for a real username
- **Fix:** SSE `data:` line parsing now correctly strips the space after the colon per the SSE specification
- **Feature:** Added Deno send client (`deno-client/send.ts`) with feature parity to `grunt send`
- Integrated Deno client into test orchestration with dedicated tmux window

## v0.4.0 — SSE Migration
- Migrated from WebSocket to SSE for pub/sub messaging
- Added `POST /api/chat/message` endpoint for REST-based sends
- Merged `recv` and `repl` into split-pane clients window

## v0.3.0 — Test Harness & Invite System
- Added automated test environment orchestrator (`scripts/test-env.sh`)
- Added invite code system with 10-minute expiration
- Fixed data race in integration tests

## v0.2.0 — API Organization & Auth
- Added OpenAPI codegen for server types
- Added auth middleware for protected endpoints
- Organized API endpoints under `/api/` prefix
- Reorganized server and storage packages
- Updated client library to match new API structure

## v0.1.0 — Initial Release
- Restructured client library into `client/` package
- Added embedded web UI (`chat.html`) with auth and chat views *(later removed in v0.2.0 refactoring)*
