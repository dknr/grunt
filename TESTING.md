# Testing Guide

## Prerequisites

1. **Always** start the server using the test environment script:
   ```bash
   cd src/grunt && bash scripts/test-env.sh
   ```
   This starts the server in a tmux session along with all test clients.

   **Never** run `./dist/grunt serve` directly.

2. View the server logs:
   ```bash
   tmux attach -t grunt-test
   ```

3. Clean up:
   ```bash
   tmux kill-session -t grunt-test
   ```

## Manual Web UI Testing

1. Start the server:
   ```bash
   cd src/grunt && bash scripts/test-env.sh
   ```

2. Note the web UI credentials printed to stdout:
   ```
   Web UI user: web / webpass
   ```

3. Open `http://localhost:54765/` in your browser.

4. Login with the credentials above.

5. The chat page will appear. Send messages from other clients (recv, repl, igor) and watch them appear in real-time.

## Automated Test Environment

Run the full test suite:
```bash
cd src/grunt && bash scripts/test-env.sh
```

This starts a tmux session with:
- Server logs
- recv/repl clients
- Two igor LLM bots
- Deno send client

View with: `tmux attach -t grunt-test`
Clean up: `tmux kill-session -t grunt-test`

## Troubleshooting

- **500 errors on templates**: Check that `templates/` files exist and are embedded
- **SSE not working**: Verify cookie is being sent (check browser DevTools)
- **Static files 404**: Ensure `static/` directory structure matches embed pattern
- **Auth failures**: Check `ExtractToken()` precedence: header → cookie → query param