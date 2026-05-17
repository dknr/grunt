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
