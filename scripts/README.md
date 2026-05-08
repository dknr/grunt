# Grunt Test Environment

Automated orchestration script for launching and managing a full grunt test environment.

## Quick Start

From the `src/grunt` directory:

```bash
./scripts/test-env.sh
```

This will:
1. Build `grunt` and `igor` binaries
2. Create a tmux session named `grunt-test`
3. Start the server, register users, and generate invite codes automatically
4. Launch `recv`, `repl`, and two `igor` instances across separate tmux windows

## Interacting with the Environment

### Attach to the tmux session
```bash
tmux attach -t grunt-test
```

### Navigate between windows
Inside tmux, use `Ctrl+b` then `1-5` to switch between windows:
- **Window 0 (server):** Grunt server logs and initial invite code
- **Window 1 (recv):** Message receiver
- **Window 2 (igor1):** First LLM bot
- **Window 3 (igor2):** Second LLM bot
- **Window 4 (repl):** Interactive REPL for sending messages

### Detach without killing
Press `Ctrl+b` then `d` to detach while keeping all processes running.

### Kill the environment
```bash
tmux kill-session -t grunt-test
```

## Debugging

### Read server logs
```bash
tmux capture-pane -t grunt-test:server -p
```

### Read logs from a specific window
```bash
tmux capture-pane -t grunt-test:recv -p
tmux capture-pane -t grunt-test:igor1 -p
tmux capture-pane -t grunt-test:igor2 -p
tmux capture-pane -t grunt-test:repl -p
```

### View all windows and panes
```bash
tmux list-windows -t grunt-test
tmux list-panes -t grunt-test
```

### Capture all output for debugging
```bash
for window in $(tmux list-windows -t grunt-test -F '#{window_index}'); do
    echo "=== Window $window ==="
    tmux capture-pane -t "grunt-test:$window" -p
done
```

### Check running processes
```bash
ps aux | grep -E "(grunt|igor)" | grep -v grep
```

## Architecture

The script handles the invite code chain automatically:
1. Server starts and logs the initial invite code
2. `recv` user is registered with the initial code
3. New invite codes are generated via HTTP API for each subsequent client
4. All clients launch with valid, non-expired invite codes

This eliminates the manual copy-paste steps that would otherwise be required.