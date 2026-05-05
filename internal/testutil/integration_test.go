package testutil

import (
	"bytes"
	"fmt"
	"net"
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

func TestIntegrationMessageFlow(t *testing.T) {
	t.Parallel()

	// Create a temporary file for the SQLite database
	tmpDir, err := os.MkdirTemp("", "grunt-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dbPath := tmpDir + "/test.db"

	// Build the grunt binary
	binPath := tmpDir + "/grunt"
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/grunt")
	buildCmd.Dir = "../.."
	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build grunt: %v\n%s", err, buildOut.String())
	}

	// Get a free port
	port := getFreePort(t)
	serverAddr := fmt.Sprintf("http://localhost:%d", port)

	// Start server subprocess
	serverCmd := exec.Command(binPath, "serve", "--port", fmt.Sprintf("%d", port), dbPath)
	var serverStdout, serverStderr bytes.Buffer
	serverCmd.Stdout = &serverStdout
	serverCmd.Stderr = &serverStderr
	if err := serverCmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Wait for server to be ready
	time.Sleep(500 * time.Millisecond)

	// Start recv subprocess (long-running)
	recvCmd := exec.Command(binPath, "recv", "--server", serverAddr, "testuser")
	var recvStdout, recvStderr bytes.Buffer
	recvCmd.Stdout = &recvStdout
	recvCmd.Stderr = &recvStderr
	if err := recvCmd.Start(); err != nil {
		t.Fatalf("Failed to start recv: %v", err)
	}

	// Wait for recv to connect
	time.Sleep(200 * time.Millisecond)

	// Send a message
	sendCmd := exec.Command(binPath, "send", "--server", serverAddr, "testuser", "Hello from integration test!")
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

	// Kill server and wait for it to fully exit
	serverCmd.Process.Kill()
	serverCmd.Wait()

	// Now it's safe to read the buffers (no more writes happening)
	serverOutput := serverStdout.String()
	recvOutput := recvStdout.String()

	if !strings.Contains(serverOutput, "Starting grunt server") {
		t.Fatalf("Server failed to start.\nServer stdout:\n%s\nServer stderr:\n%s", serverOutput, serverStderr.String())
	}

	if !strings.Contains(recvOutput, "Hello from integration test!") {
		t.Errorf("recv did not receive the message.\nrecv stdout:\n%s\nrecv stderr:\n%s", recvOutput, recvStderr.String())
	}
}
