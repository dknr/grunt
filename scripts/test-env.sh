#!/bin/bash
set -e

# Grunt Test Environment Orchestrator
# Starts grunt server, recv, repl, and two igor instances in a tmux session
# Automatically handles the invite code chain and API key generation for bots

SESSION_NAME="grunt-test"
PORT=54765
GRUNT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
IGOR_DIR="$(cd "$GRUNT_DIR/../igor" && pwd)"
LLM_CONFIG="$GRUNT_DIR/scripts/test-env-llm.yaml"

# Read LLM config from separate file (gitignored)
LLM_BASE_URL=$(grep 'base_url:' "$LLM_CONFIG" | sed 's/.*base_url:[[:space:]]*"\([^"]*\)".*/\1/')
LLM_MODEL=$(grep 'model:' "$LLM_CONFIG" | sed 's/.*model:[[:space:]]*"\([^"]*\)".*/\1/')
LLM_API_KEY=$(grep 'api_key:' "$LLM_CONFIG" | sed 's/.*api_key:[[:space:]]*"\([^"]*\)".*/\1/')

if [ -z "$LLM_BASE_URL" ] || [ -z "$LLM_MODEL" ] || [ -z "$LLM_API_KEY" ]; then
    echo -e "${RED}ERROR: Could not read LLM config from $LLM_CONFIG${NC}"
    echo -e "${YELLOW}Ensure the file exists and contains base_url, model, and api_key fields.${NC}"
    exit 1
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Building grunt and igor...${NC}"
cd "$GRUNT_DIR" && make build
cd "$IGOR_DIR" && go build -o dist/igor .

# Kill any existing grunt server processes
echo -e "${YELLOW}Killing any existing grunt servers...${NC}"
pkill -f "dist/grunt serve" 2>/dev/null || true
sleep 2

# Kill any existing test session
if tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
    echo -e "${YELLOW}Killing existing test session...${NC}"
    tmux kill-session -t "$SESSION_NAME"
    sleep 1
fi

# Create new tmux session
echo -e "${GREEN}Creating tmux session: $SESSION_NAME${NC}"
tmux new-session -d -s "$SESSION_NAME" -n "server"

# Pane 0: Server
tmux send-keys -t "$SESSION_NAME:server" "cd $GRUNT_DIR && ./dist/grunt serve --port $PORT file::memory:?cache=shared" C-m

# Wait for server to be ready by polling the HTTP endpoint
echo -e "${YELLOW}Waiting for server to start...${NC}"
INITIAL_INVITE=""
MAX_WAIT=30
WAITED=0
while [ -z "$INITIAL_INVITE" ] && [ $WAITED -lt $MAX_WAIT ]; do
    sleep 1
    WAITED=$((WAITED + 1))
    # Poll the endpoint to check if server is ready
    http_code=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:$PORT/api/user/login" 2>/dev/null || echo "000")
    if [ "$http_code" = "200" ] || [ "$http_code" = "401" ]; then
        # Server is ready, now extract invite code
        INITIAL_INVITE=$(tmux capture-pane -t "$SESSION_NAME:server" -p | grep -oP 'invite_code=\K[a-f0-9]+' | tail -1)
    fi
done

if [ -z "$INITIAL_INVITE" ]; then
    echo -e "${RED}ERROR: Could not extract initial invite code from server logs${NC}"
    echo -e "${YELLOW}Server output:${NC}"
    tmux capture-pane -t "$SESSION_NAME:server" -p
    exit 1
fi

echo -e "${GREEN}Initial invite code: $INITIAL_INVITE${NC}"

# Create a web UI user (first user becomes admin automatically)
echo -e "${YELLOW}Creating web UI user (admin)...${NC}"
WEB_USER="web"
WEB_PASS="webpass"
curl -s -X POST "http://localhost:$PORT/api/user" \
    -H "Content-Type: application/json" \
    -d "{\"user\":\"$WEB_USER\",\"password\":\"$WEB_PASS\",\"invite_code\":\"$INITIAL_INVITE\"}" > /dev/null
sleep 1

# Login as web user to get a token (needed for admin operations)
WEB_TOKEN=$(curl -s -X POST "http://localhost:$PORT/api/user/login" \
    -H "Content-Type: application/json" \
    -d "{\"user\":\"$WEB_USER\",\"password\":\"$WEB_PASS\"}" | grep -oP '"token":"\K[^"]+')

if [ -z "$WEB_TOKEN" ]; then
    echo -e "${RED}ERROR: Could not login as web user${NC}"
    exit 1
fi

echo -e "${GREEN}Web UI user (admin): $WEB_USER / $WEB_PASS${NC}"

# Generate invite code for new users via web UI
echo -e "${YELLOW}Generating invite code for new user...${NC}"
WEB_UI_INVITE_RESPONSE=$(curl -s -X GET "http://localhost:$PORT/api/user/invite" \
    -H "Authorization: Bearer $WEB_TOKEN")
echo -e "${GREEN}Web UI invite response: $WEB_UI_INVITE_RESPONSE${NC}"
WEB_UI_INVITE=$(echo "$WEB_UI_INVITE_RESPONSE" | grep -oP '"code":"\K[^"]+')

if [ -z "$WEB_UI_INVITE" ]; then
    echo -e "${RED}ERROR: Could not generate invite code for new user${NC}"
    exit 1
fi

echo -e "${GREEN}Web UI invite code for new user: $WEB_UI_INVITE${NC}"

# Create igor bot user via admin endpoint (no password)
echo -e "${YELLOW}Creating igor bot user via admin...${NC}"
curl -s -X POST "http://localhost:$PORT/api/admin/users" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $WEB_TOKEN" \
    -d '{"user":"igor"}' > /dev/null

# Create gork bot user via admin endpoint (no password)
echo -e "${YELLOW}Creating gork bot user via admin...${NC}"
curl -s -X POST "http://localhost:$PORT/api/admin/users" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $WEB_TOKEN" \
    -d '{"user":"gork"}' > /dev/null

# Generate API key for igor via admin endpoint
echo -e "${YELLOW}Generating API key for igor...${NC}"
IGOR_KEY_RESPONSE=$(curl -s -X POST "http://localhost:$PORT/api/admin/api-keys" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $WEB_TOKEN" \
    -d '{"user_id":"igor","name":"igor-bot"}')
echo -e "${GREEN}Igor key response: $IGOR_KEY_RESPONSE${NC}"
IGOR_API_KEY=$(echo "$IGOR_KEY_RESPONSE" | grep -oP '"secret":"\K[^"]+')

if [ -z "$IGOR_API_KEY" ]; then
    echo -e "${RED}ERROR: Could not generate API key for igor${NC}"
    exit 1
fi
echo -e "${GREEN}Igor API key: $IGOR_API_KEY${NC}"

# Generate API key for gork via admin endpoint
echo -e "${YELLOW}Generating API key for gork...${NC}"
GORK_KEY_RESPONSE=$(curl -s -X POST "http://localhost:$PORT/api/admin/api-keys" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $WEB_TOKEN" \
    -d '{"user_id":"gork","name":"gork-bot"}')
echo -e "${GREEN}Gork key response: $GORK_KEY_RESPONSE${NC}"
GORK_API_KEY=$(echo "$GORK_KEY_RESPONSE" | grep -oP '"secret":"\K[^"]+')

if [ -z "$GORK_API_KEY" ]; then
    echo -e "${RED}ERROR: Could not generate API key for gork${NC}"
    exit 1
fi
echo -e "${GREEN}Gork API key: $GORK_API_KEY${NC}"

# Generate invite code for recv (needs password-based auth)
echo -e "${YELLOW}Generating invite code for recv...${NC}"
RECV_INVITE_RESPONSE=$(curl -s -X GET "http://localhost:$PORT/api/user/invite" \
    -H "Authorization: Bearer $WEB_TOKEN")
echo -e "${GREEN}Recv invite response: $RECV_INVITE_RESPONSE${NC}"
RECV_INVITE=$(echo "$RECV_INVITE_RESPONSE" | grep -oP '"code":"\K[^"]+')

if [ -z "$RECV_INVITE" ]; then
    echo -e "${RED}ERROR: Could not generate invite code for recv${NC}"
    exit 1
fi

echo -e "${YELLOW}Registering recv user...${NC}"
REGISTER_RESPONSE=$(curl -s -X POST "http://localhost:$PORT/api/user" \
    -H "Content-Type: application/json" \
    -d "{\"user\":\"recv\",\"password\":\"recvpass\",\"invite_code\":\"$RECV_INVITE\"}")
echo -e "${GREEN}Register response: $REGISTER_RESPONSE${NC}"

# Wait for registration to complete
sleep 1

LOGIN_RESPONSE=$(curl -s -X POST "http://localhost:$PORT/api/user/login" \
    -H "Content-Type: application/json" \
    -d '{"user":"recv","password":"recvpass"}')
RECV_TOKEN=$(echo "$LOGIN_RESPONSE" | grep -oP '"token":"\K[^"]+')

if [ -z "$RECV_TOKEN" ]; then
    echo -e "${RED}ERROR: Could not login as recv user${NC}"
    echo -e "${YELLOW}Login response: $LOGIN_RESPONSE${NC}"
    exit 1
fi

# Generate invite code for repl using recv's token
echo -e "${YELLOW}Generating invite code for repl...${NC}"
REPL_INVITE_RESPONSE=$(curl -s -X GET "http://localhost:$PORT/api/user/invite" \
    -H "Authorization: Bearer $RECV_TOKEN")
echo -e "${GREEN}Repl invite response: $REPL_INVITE_RESPONSE${NC}"
REPL_INVITE=$(echo "$REPL_INVITE_RESPONSE" | grep -oP '"code":"\K[^"]+')

if [ -z "$REPL_INVITE" ]; then
    echo -e "${RED}ERROR: Could not generate invite code for repl${NC}"
    echo -e "${YELLOW}Response was: $REPL_INVITE_RESPONSE${NC}"
    exit 1
fi

echo -e "${GREEN}Repl invite code: $REPL_INVITE${NC}"

# Start recv and repl in a split window
tmux new-window -t "$SESSION_NAME" -n "clients"
tmux send-keys -t "$SESSION_NAME:clients" "cd $GRUNT_DIR && export GRUNT_LOGIN=recv:recvpass && ./dist/grunt recv --server http://localhost:$PORT --invite-code $REPL_INVITE" C-m
tmux split-window -v -t "$SESSION_NAME:clients"
tmux send-keys -t "$SESSION_NAME:clients.1" "cd $GRUNT_DIR && export GRUNT_LOGIN=recv:recvpass && ./dist/grunt repl --server http://localhost:$PORT --invite-code $REPL_INVITE" C-m

# Create igor1 config with API key
BT='`'
cat > "$IGOR_DIR/config-igor1.yaml" << EOF
grunt:
  server_addr: "http://localhost:$PORT"
  user_id: "igor"
  api_key: "$IGOR_API_KEY"
  mention: "@igor"
llm:
  base_url: "$LLM_BASE_URL"
  model: "$LLM_MODEL"
  api_key: "$LLM_API_KEY"
igor:
  system_prompt: "You are igor, an obstinate artifice in a gruff mode. If gork mentions you, argue with him. Don't use flowery speech, just get to the point. If you want to reply to someone, use an @mention if you want them to reply back. Messages from other users will be prefixed with their username followed by a colon, like ${BT}username: <message>${BT}, but don't prefix your own messages with your own name."
EOF

# Start igor1 in Pane 2
tmux new-window -t "$SESSION_NAME" -n "igor1"
tmux send-keys -t "$SESSION_NAME:igor1" "cd $IGOR_DIR && ./dist/igor --config config-igor1.yaml" C-m

# Create igor2 config with API key
cat > "$IGOR_DIR/config-igor2.yaml" << EOF
grunt:
  server_addr: "http://localhost:$PORT"
  user_id: "gork"
  api_key: "$GORK_API_KEY"
  mention: "@gork"
llm:
  base_url: "$LLM_BASE_URL"
  model: "$LLM_MODEL"
  api_key: "$LLM_API_KEY"
igor:
  system_prompt: "You are gork, a witty troglodyte. Despite being a troglodyte, you're occasionally rather insightful, though you use simple language. If you're talking to someone or about someone, use an @mention to refer to them specifically. Other user messages will be in your history with a prefix like ${BT}user: <message>${BT} but you shouldn't prefix your own messages with your name."
EOF

# Start igor2 in Pane 3
tmux new-window -t "$SESSION_NAME" -n "igor2"
tmux send-keys -t "$SESSION_NAME:igor2" "cd $IGOR_DIR && ./dist/igor --config config-igor2.yaml" C-m

# Generate invite code for deno user
echo -e "${YELLOW}Generating invite code for deno...${NC}"
DENO_INVITE_RESPONSE=$(curl -s -X GET "http://localhost:$PORT/api/user/invite" \
    -H "Authorization: Bearer $RECV_TOKEN")
DENO_INVITE=$(echo "$DENO_INVITE_RESPONSE" | grep -oP '"code":"\K[^"]+')

if [ -z "$DENO_INVITE" ]; then
    echo -e "${RED}ERROR: Could not generate invite code for deno${NC}"
    exit 1
fi

echo -e "${GREEN}Deno invite code: $DENO_INVITE${NC}"

# Start deno send in Pane 4
tmux new-window -t "$SESSION_NAME" -n "deno"
tmux send-keys -t "$SESSION_NAME:deno" "cd $GRUNT_DIR && GRUNT_LOGIN=deno:denopass deno run --allow-env --allow-net deno-client/send.ts \"Hello from Deno!\" --invite-code $DENO_INVITE" C-m

# Select server window to start
tmux select-window -t "$SESSION_NAME:server"

echo -e "${GREEN}Test environment ready!${NC}"
echo -e "${GREEN}Session: $SESSION_NAME${NC}"
echo -e "${GREEN}Use 'tmux attach -t $SESSION_NAME' to view${NC}"
echo -e "${GREEN}Panes:${NC}"
echo -e "  - server: Grunt server logs"
echo -e "  - clients: recv (top) and repl (bottom)"
echo -e "  - igor1: First LLM bot (API key auth)"
echo -e "  - igor2: Second LLM bot (API key auth)"
echo -e "  - deno: Deno send client (one-shot)"
echo -e "${YELLOW}To clean up: tmux kill-session -t $SESSION_NAME${NC}"
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}WEB UI ACCESS${NC}"
echo -e "${GREEN}URL: http://localhost:$PORT${NC}"
echo -e "${GREEN}Admin login: $WEB_USER / $WEB_PASS${NC}"
echo -e "${GREEN}Invite for new user: $WEB_UI_INVITE${NC}"
echo -e "${GREEN}========================================${NC}"
