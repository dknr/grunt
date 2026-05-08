package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

// getFreePort finds a free port on the local machine.
func getFreePort(t *testing.T) int {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to resolve address: %v", err)
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to listen on port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func buildBinary(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "grunt-build-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	binPath := tmpDir + "/grunt"
	buildCmd := exec.Command("go", "build", "-o", binPath, "./grunt")
	buildCmd.Dir = "../.."
	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build grunt: %v\n%s", err, buildOut.String())
	}
	return binPath
}

type serverInfo struct {
	cmd           *exec.Cmd
	addr          string
	dbPath        string
	stdout        string // path to file containing stdout
	stderr        string // path to file containing stderr
	initialInvite string
}

func startServer(t *testing.T, binPath string, port int) *serverInfo {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "grunt-server-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dbPath := tmpDir + "/test.db"
	serverAddr := fmt.Sprintf("http://localhost:%d", port)

	stdoutFile, err := os.CreateTemp(tmpDir, "stdout-*.log")
	if err != nil {
		t.Fatalf("Failed to create stdout file: %v", err)
	}
	stdoutPath := stdoutFile.Name()
	stdoutFile.Close()

	stderrFile, err := os.CreateTemp(tmpDir, "stderr-*.log")
	if err != nil {
		t.Fatalf("Failed to create stderr file: %v", err)
	}
	stderrPath := stderrFile.Name()
	stderrFile.Close()

	serverCmd := exec.Command(binPath, "serve", "--port", fmt.Sprintf("%d", port), dbPath)
	serverCmd.Stdout, _ = os.OpenFile(stdoutPath, os.O_WRONLY|os.O_APPEND, 0644)
	serverCmd.Stderr, _ = os.OpenFile(stderrPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err := serverCmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Poll the HTTP endpoint to detect when the server is ready.
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			stderrContent, _ := os.ReadFile(stderrPath)
			t.Fatalf("Server did not start within timeout.\nStderr:\n%s", stderrContent)
		case <-ticker.C:
			resp, err := http.Get(serverAddr + "/api/user/login")
			if err == nil {
				resp.Body.Close()
				// Server is ready. Read the invite code from the output files.
				// Files are safe to read because the server's startup phase is complete.
				stdoutContent, _ := os.ReadFile(stdoutPath)
				stderrContent, _ := os.ReadFile(stderrPath)
				initialInvite := extractInviteCode(string(stdoutContent))
				if initialInvite == "" {
					initialInvite = extractInviteCode(string(stderrContent))
				}
				return &serverInfo{
					cmd:           serverCmd,
					addr:          serverAddr,
					dbPath:        dbPath,
					stdout:        stdoutPath,
					stderr:        stderrPath,
					initialInvite: initialInvite,
				}
			}
		}
	}
}

// extractInviteCode searches server output for the initial invite code.
func extractInviteCode(output string) string {
	re := regexp.MustCompile(`invite_code=([a-f0-9]+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// getInviteCode makes an authenticated request to generate a new invite code.
func getInviteCode(t *testing.T, serverAddr, token string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, serverAddr+"/api/user/invite", nil)
	if err != nil {
		t.Fatalf("Failed to create invite request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to get invite: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode invite response: %v", err)
	}
	return result.Code
}

// loginAndGetToken authenticates a user and returns the token.
func loginAndGetToken(t *testing.T, serverAddr, user, password string) string {
	t.Helper()
	resp, err := http.Post(serverAddr+"/api/user/login", "application/json",
		bytes.NewReader([]byte(fmt.Sprintf(`{"user":"%s","password":"%s"}`, user, password))))
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}
	return result.Token
}

func stopServer(t *testing.T, info *serverInfo) {
	t.Helper()
	info.cmd.Process.Kill()
	info.cmd.Wait()
}

func TestIntegrationMessageFlow(t *testing.T) {
	t.Parallel()

	binPath := buildBinary(t)
	port := getFreePort(t)
	serverInfo := startServer(t, binPath, port)

	// Register first user (alice) with initial invite code
	resp, err := http.Post(serverInfo.addr+"/api/user", "application/json",
		bytes.NewReader([]byte(fmt.Sprintf(`{"user":"alice","password":"password","invite_code":"%s"}`, serverInfo.initialInvite))))
	if err != nil {
		t.Fatalf("Failed to register alice: %v", err)
	}
	resp.Body.Close()

	// Login as alice to get a token for generating new invites
	token := loginAndGetToken(t, serverInfo.addr, "alice", "password")

	// Generate invite codes for bob's recv and send subprocesses
	bobRecvInvite := getInviteCode(t, serverInfo.addr, token)
	bobSendInvite := getInviteCode(t, serverInfo.addr, token)

	t.Logf("Generated bobRecvInvite: %s, bobSendInvite: %s", bobRecvInvite, bobSendInvite)

	// Register bob with recv invite code
	resp, err = http.Post(serverInfo.addr+"/api/user", "application/json",
		bytes.NewReader([]byte(fmt.Sprintf(`{"user":"bob","password":"password","invite_code":"%s"}`, bobRecvInvite))))
	if err != nil {
		t.Fatalf("Failed to register bob: %v", err)
	}
	resp.Body.Close()

	// Start bob's recv subprocess (long-running)
	recvCmd := exec.Command(binPath, "recv", "--server", serverInfo.addr, "--invite-code", bobSendInvite)
	recvCmd.Env = append(os.Environ(), "GRUNT_LOGIN=bob:password")
	var recvStdout, recvStderr bytes.Buffer
	recvCmd.Stdout = &recvStdout
	recvCmd.Stderr = &recvStderr
	if err := recvCmd.Start(); err != nil {
		t.Fatalf("Failed to start recv for bob: %v", err)
	}

	// Wait for bob to connect
	time.Sleep(200 * time.Millisecond)

	// Login as alice and generate a new invite for send
	aliceSendInvite := getInviteCode(t, serverInfo.addr, token)

	// Send a message from alice to bob
	sendCmd := exec.Command(binPath, "send", "--server", serverInfo.addr, "--invite-code", aliceSendInvite, "Hello from alice to bob!")
	sendCmd.Env = append(os.Environ(), "GRUNT_LOGIN=alice:password")
	var sendStdout, sendStderr bytes.Buffer
	sendCmd.Stdout = &sendStdout
	sendCmd.Stderr = &sendStderr
	if err := sendCmd.Run(); err != nil {
		t.Logf("Send command output: %s\n%s", sendStdout.String(), sendStderr.String())
	}

	// Wait for bob to process the message
	time.Sleep(500 * time.Millisecond)

	// Force kill bob's recv (it runs indefinitely)
	recvCmd.Process.Kill()
	recvCmd.Wait()

	// Stop server before reading its output to avoid data race
	stopServer(t, serverInfo)

	recvOutput := recvStdout.String()
	stdoutContent, _ := os.ReadFile(serverInfo.stdout)
	stderrContent, _ := os.ReadFile(serverInfo.stderr)
	serverOutput := stdoutContent

	if !strings.Contains(string(serverOutput), "Starting grunt server") {
		t.Fatalf("Server failed to start.\nServer stdout:\n%s\nServer stderr:\n%s",
			string(serverOutput), string(stderrContent))
	}

	if !strings.Contains(recvOutput, "Hello from alice to bob!") {
		t.Errorf("Bob did not receive alice's message.\nrecv stdout:\n%s\nrecv stderr:\n%s", recvOutput, recvStderr.String())
	}
}

func TestIntegrationAuthRegistration(t *testing.T) {
	t.Parallel()

	binPath := buildBinary(t)
	port := getFreePort(t)
	serverInfo := startServer(t, binPath, port)
	defer stopServer(t, serverInfo)

	// Test registration with invite code
	resp, err := http.Post(serverInfo.addr+"/api/user", "application/json",
		bytes.NewReader([]byte(fmt.Sprintf(`{"user":"authuser","password":"testpass123","invite_code":"%s"}`, serverInfo.initialInvite))))
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 201, got %d. Body: %s", resp.StatusCode, string(body))
	}

	// Verify user exists via login
	loginResp, err := http.Post(serverInfo.addr+"/api/user/login", "application/json",
		bytes.NewReader([]byte(fmt.Sprintf(`{"user":"authuser","password":"testpass123","invite_code":"%s"}`, serverInfo.initialInvite))))
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(loginResp.Body)
		t.Fatalf("Expected 200, got %d. Body: %s", loginResp.StatusCode, string(body))
	}

	var loginResult struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(loginResp.Body).Decode(&loginResult); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}
	if loginResult.Token == "" {
		t.Fatal("Expected non-empty token")
	}
}

func TestIntegrationAuthWrongPassword(t *testing.T) {
	t.Parallel()

	binPath := buildBinary(t)
	port := getFreePort(t)
	serverInfo := startServer(t, binPath, port)
	defer stopServer(t, serverInfo)

	// Register a user with invite code
	resp, err := http.Post(serverInfo.addr+"/api/user", "application/json",
		bytes.NewReader([]byte(fmt.Sprintf(`{"user":"wrongpassuser","password":"correctpass","invite_code":"%s"}`, serverInfo.initialInvite))))
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201, got %d", resp.StatusCode)
	}

	// Try to login with wrong password
	loginResp, err := http.Post(serverInfo.addr+"/api/user/login", "application/json",
		bytes.NewReader([]byte(`{"user":"wrongpassuser","password":"wrongpass"}`)))
	if err != nil {
		t.Fatalf("Failed to attempt login: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(loginResp.Body)
		t.Fatalf("Expected 401, got %d. Body: %s", loginResp.StatusCode, string(body))
	}
}

func TestIntegrationAuthTwoUsers(t *testing.T) {
	t.Parallel()

	binPath := buildBinary(t)
	port := getFreePort(t)
	serverInfo := startServer(t, binPath, port)

	// Register first user (alice) with initial invite code
	resp, err := http.Post(serverInfo.addr+"/api/user", "application/json",
		bytes.NewReader([]byte(fmt.Sprintf(`{"user":"alice","password":"password","invite_code":"%s"}`, serverInfo.initialInvite))))
	if err != nil {
		t.Fatalf("Failed to register alice: %v", err)
	}
	resp.Body.Close()

	// Login as alice to get a token
	aliceToken := loginAndGetToken(t, serverInfo.addr, "alice", "password")

	// Generate invite codes: one for alice's recv, one for bob's registration
	aliceRecvInvite := getInviteCode(t, serverInfo.addr, aliceToken)
	bobInvite := getInviteCode(t, serverInfo.addr, aliceToken)

	t.Logf("Generated aliceRecvInvite: %s, bobInvite: %s", aliceRecvInvite, bobInvite)

	// Register bob with the invite code
	resp, err = http.Post(serverInfo.addr+"/api/user", "application/json",
		bytes.NewReader([]byte(fmt.Sprintf(`{"user":"bob","password":"password","invite_code":"%s"}`, bobInvite))))
	if err != nil {
		t.Fatalf("Failed to register bob: %v", err)
	}
	resp.Body.Close()

	// Login as bob to get a token and another invite for sending
	bobToken := loginAndGetToken(t, serverInfo.addr, "bob", "password")
	bobSendInvite := getInviteCode(t, serverInfo.addr, bobToken)

	t.Logf("Generated bobSendInvite: %s", bobSendInvite)

	// Start recv for alice (using a valid invite code)
	recvCmd := exec.Command(binPath, "recv", "--server", serverInfo.addr, "--invite-code", aliceRecvInvite)
	recvCmd.Env = append(os.Environ(), "GRUNT_LOGIN=alice:password")
	var recvStdout, recvStderr bytes.Buffer
	recvCmd.Stdout = &recvStdout
	recvCmd.Stderr = &recvStderr
	if err := recvCmd.Start(); err != nil {
		t.Fatalf("Failed to start recv for alice: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Send message from bob
	sendCmd := exec.Command(binPath, "send", "--server", serverInfo.addr, "--invite-code", bobSendInvite, "Hello from bob to alice!")
	sendCmd.Env = append(os.Environ(), "GRUNT_LOGIN=bob:password")
	var sendStdout, sendStderr bytes.Buffer
	sendCmd.Stdout = &sendStdout
	sendCmd.Stderr = &sendStderr
	if err := sendCmd.Run(); err != nil {
		t.Logf("Send output: %s\n%s", sendStdout.String(), sendStderr.String())
	}

	time.Sleep(500 * time.Millisecond)

	recvCmd.Process.Kill()
	recvCmd.Wait()

	// Stop server before reading its output
	stopServer(t, serverInfo)

	recvOutput := recvStdout.String()
	if !strings.Contains(recvOutput, "Hello from bob to alice!") {
		t.Errorf("Alice did not receive bob's message.\nrecv stdout:\n%s\nrecv stderr:\n%s", recvOutput, recvStderr.String())
	}
}
