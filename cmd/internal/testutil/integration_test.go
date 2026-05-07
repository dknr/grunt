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
	cmd        *exec.Cmd
	addr       string
	dbPath     string
	stdout     *bytes.Buffer
	stderr     *bytes.Buffer
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

	serverCmd := exec.Command(binPath, "serve", "--port", fmt.Sprintf("%d", port), dbPath)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	serverCmd.Stdout = stdout
	serverCmd.Stderr = stderr
	if err := serverCmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Wait for server to be ready
	time.Sleep(500 * time.Millisecond)

	return &serverInfo{
		cmd:    serverCmd,
		addr:   serverAddr,
		dbPath: dbPath,
		stdout: stdout,
		stderr: stderr,
	}
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

	// Start recv subprocess (long-running)
	recvCmd := exec.Command(binPath, "recv", "--server", serverInfo.addr)
	recvCmd.Env = append(os.Environ(), "GRUNT_LOGIN=testuser:password")
	var recvStdout, recvStderr bytes.Buffer
	recvCmd.Stdout = &recvStdout
	recvCmd.Stderr = &recvStderr
	if err := recvCmd.Start(); err != nil {
		t.Fatalf("Failed to start recv: %v", err)
	}

	// Wait for recv to connect
	time.Sleep(200 * time.Millisecond)

	// Send a message
	sendCmd := exec.Command(binPath, "send", "--server", serverInfo.addr, "Hello from integration test!")
	sendCmd.Env = append(os.Environ(), "GRUNT_LOGIN=testuser:password")
	var sendStdout, sendStderr bytes.Buffer
	sendCmd.Stdout = &sendStdout
	sendCmd.Stderr = &sendStderr
	if err := sendCmd.Run(); err != nil {
		t.Logf("Send command output: %s\n%s", sendStdout.String(), sendStderr.String())
	}

	// Wait for recv to process the message
	time.Sleep(500 * time.Millisecond)

	// Force kill recv (it runs indefinitely)
	recvCmd.Process.Kill()
	recvCmd.Wait()

	// Stop server before reading its output to avoid data race
	stopServer(t, serverInfo)

	recvOutput := recvStdout.String()
	serverOutput := serverInfo.stdout.String()

	if !strings.Contains(serverOutput, "Starting grunt server") {
		t.Fatalf("Server failed to start.\nServer stdout:\n%s\nServer stderr:\n%s",
			serverOutput, serverInfo.stderr.String())
	}

	if !strings.Contains(recvOutput, "Hello from integration test!") {
		t.Errorf("recv did not receive the message.\nrecv stdout:\n%s\nrecv stderr:\n%s", recvOutput, recvStderr.String())
	}
}

func TestIntegrationAuthRegistration(t *testing.T) {
	t.Parallel()

	binPath := buildBinary(t)
	port := getFreePort(t)
	serverInfo := startServer(t, binPath, port)
	defer stopServer(t, serverInfo)

	// Test registration with password
	resp, err := http.Post(serverInfo.addr+"/api/user", "application/json",
		bytes.NewReader([]byte(`{"user":"authuser","password":"testpass123"}`)))
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
		bytes.NewReader([]byte(`{"user":"authuser","password":"testpass123"}`)))
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

	// Register a user
	resp, err := http.Post(serverInfo.addr+"/api/user", "application/json",
		bytes.NewReader([]byte(`{"user":"wrongpassuser","password":"correctpass"}`)))
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

	// Register two users with password "password" (matching the GRUNT_LOGIN env var)
	for _, user := range []string{"alice", "bob"} {
		resp, err := http.Post(serverInfo.addr+"/api/user", "application/json",
			bytes.NewReader([]byte(fmt.Sprintf(`{"user":"%s","password":"password"}`, user))))
		if err != nil {
			t.Fatalf("Failed to register %s: %v", user, err)
		}
		resp.Body.Close()
	}

	// Start recv for alice
	recvCmd := exec.Command(binPath, "recv", "--server", serverInfo.addr)
	recvCmd.Env = append(os.Environ(), "GRUNT_LOGIN=alice:password")
	var recvStdout, recvStderr bytes.Buffer
	recvCmd.Stdout = &recvStdout
	recvCmd.Stderr = &recvStderr
	if err := recvCmd.Start(); err != nil {
		t.Fatalf("Failed to start recv for alice: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Send message from bob
	sendCmd := exec.Command(binPath, "send", "--server", serverInfo.addr, "Hello from bob to alice!")
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
		t.Errorf("Alice did not receive bob's message.\nrecv stdout:\n%s", recvOutput)
	}
}
