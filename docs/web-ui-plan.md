# Plan: Simple Web UI for Grunt

This plan focuses on a single-file HTML/JS solution embedded directly into the Go server. It avoids external build tools and libraries, sticking to the "1990s simple" aesthetic.

## 1. File Structure

*   **New File**: `src/grunt/cmd/internal/server/chat.html`
    *   Contains the entire UI: HTML structure, inline CSS, and vanilla JavaScript.
*   **Modified File**: `src/grunt/cmd/internal/server/server.go`
    *   Adds `//go:embed chat.html` directive.
    *   Adds new HTTP handlers for `/login`, `/register`, and `/chat`.

## 2. UI Design (`chat.html`)

The page will have two views controlled by JavaScript: **Auth View** and **Chat View**.

### Auth View
*   A centered form container.
*   **Fields**:
    1.  `Username` (text input)
    2.  `Password` (password input)
    3.  `Invite Code` (text input, ignored by server)
*   **Buttons**:
    1.  `[Login]`
    2.  `[Register]`
*   **Layout**: Vertical stack (`display: flex; flex-direction: column;`).
*   **Logic**: Clicking a button updates the form's `action` attribute (`/login` or `/register`) before submission.

### Chat View (Hidden initially)
*   **Message List**: A `div` or `ul` that appends incoming messages.
*   **Input Area**: Text input + `[Send]` button.
*   **Logic**:
    *   On load, checks for a `session` cookie.
    *   If cookie exists: Hides Auth View, shows Chat View, connects to WebSocket.
    *   If no cookie: Shows Auth View.

## 3. Server Logic (`server.go`)

*   **`GET /chat`**:
    *   Serves the embedded `chat.html` file.
*   **`POST /login`**:
    *   Parses form data (`user`, `password`).
    *   Calls `store.VerifyUser`.
    *   If valid:
        *   Generates a session token.
        *   Sets a cookie `session=<token>` (path `/`, max-age 24h).
        *   Returns **303 See Other** redirect to `/chat`.
*   **`POST /register`**:
    *   Parses form data (`user`, `password`, `invite`).
    *   Calls `store.CreateUser`.
    *   If successful:
        *   Generates a session token.
        *   Sets a cookie `session=<token>`.
        *   Returns **303 See Other** redirect to `/chat`.

## 4. WebSocket Integration

*   **Client Side**:
    *   The JS in `chat.html` will read the `session` cookie from `document.cookie`.
    *   It will construct the WebSocket URL: `ws://host/ws?token=<cookie_value>`.
*   **Server Side**:
    *   The existing `/ws` handler already supports `?token=`. No changes needed there.

## 5. Implementation Steps

1.  **Create `chat.html`**:
    *   Write the HTML/CSS/JS.
    *   Implement `setAction(url)` for the buttons.
    *   Implement `getCookie(name)` helper.
    *   Implement `connectChat(token)` function.
2.  **Update `server.go`**:
    *   Add `//go:embed chat.html`.
    *   Add `serveChat` handler.
    *   Add `handleLogin` and `handleRegister` handlers.
    *   Register routes in `setupRoutes`.
3.  **Test**:
    *   Run `grunt serve`.
    *   Navigate to `http://localhost:54765/chat`.
    *   Verify registration, login, cookie setting, and redirect.
    *   Verify WebSocket connection and message sending.

## 6. Example HTML Structure

```html
<!DOCTYPE html>
<html>
<head>
    <title>Grunt Chat</title>
    <style>
        body { font-family: sans-serif; max-width: 600px; margin: 2rem auto; }
        .hidden { display: none; }
        form { display: flex; flex-direction: column; gap: 10px; }
        input { padding: 8px; }
        button { padding: 8px; cursor: pointer; }
    </style>
</head>
<body>
    <!-- Auth View -->
    <div id="auth-view">
        <h2>Login / Register</h2>
        <form id="auth-form" method="POST">
            <input name="user" placeholder="Username" required>
            <input name="password" type="password" placeholder="Password" required>
            <input name="invite" placeholder="Invite Code">
            <button type="button" onclick="setAction('/login')">Login</button>
            <button type="button" onclick="setAction('/register')">Register</button>
        </form>
    </div>

    <!-- Chat View -->
    <div id="chat-view" class="hidden">
        <div id="messages"></div>
        <form id="chat-form" method="POST" style="flex-direction: row;">
            <input id="msg-input" name="content" placeholder="Type a message..." required>
            <button type="submit">Send</button>
        </form>
    </div>

    <script>
        // JS logic for cookie handling, WS connection, and form submission
    </script>
</body>
</html>
```

## 7. Questions

None at this time. The scope is defined and implementation steps are clear.