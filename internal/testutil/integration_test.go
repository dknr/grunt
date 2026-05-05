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
	// Build the grunt binary
	tmpDir, err := os.MkdirTemp("", "grunt-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	binPath := tmpDir + "/grunt"
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/grunt")
	buildCmd.Dir = "../.." // Assume test is run from grunt root
	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build grunt: %v\n%s", err, buildOut.String())
	}

	// Get a free port
	port := getFreePort(t)
	serverAddr := fmt.Sprintf("http://localhost:%d", port)

	// Use in-memory database (shared within the single server process)
	dbDSN := "file::memory:?cache=shared"

	// Start server subprocess
	serverCmd := exec.Command(binPath, "serve", "--port", fmt.Sprintf("%d", port), dbDSN)
	var serverStdout, serverStderr bytes.Buffer
	serverCmd.Stdout = &serverStdout
	serverCmd.Stderr = &serverStderr
	if err := serverCmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer serverCmd.Process.Kill()

	// Wait for server to be ready
	time.Sleep(1 * time.Second)

	// Check if server started successfully
	serverOutput := serverStdout.String()
	if !strings.Contains(serverOutput, "Starting grunt server") {
		t.Fatalf("Server failed to start.\nServer stdout:\n%s\nServer stderr:\n%s", serverOutput, serverStderr.String())
	}

	// Start recv subprocess
	recvCmd := exec.Command(binPath, "recv", "--server", serverAddr, "testuser")
	var recvStdout, recvStderr bytes.Buffer
	recvCmd.Stdout = &recvStdout
	recvCmd.Stderr = &recvStderr
	if err := recvCmd.Start(); err != nil {
		t.Fatalf("Failed to start recv: %v", err)
	}
	defer recvCmd.Process.Kill()

	// Wait for recv to connect
	time.Sleep(500 * time.Millisecond)

	// Send a message
	sendCmd := exec.Command(binPath, "send", "--server", serverAddr, "testuser", "Hello from integration test!")
	var sendStdout, sendStderr bytes.Buffer
	sendCmd.Stdout = &sendStdout
	sendCmd.Stderr = &sendStderr
	if err := sendCmd.Run(); err != nil {
		t.Logf("Send command output: %s\n%s", sendStdout.String(), sendStderr.String())
	}

	// Wait for recv to process the message
	time.Sleep(1 * time.Second)

	// Check if recv received the message
	recvOutput := recvStdout.String()
	if !strings.Contains(recvOutput, "Hello from integration test!") {
		t.Errorf("recv did not receive the message.\nrecv stdout:\n%s\nrecv stderr:\n%s", recvOutput, recvStderr.String())
	}

	// Message flow is verified by recv receiving the message
	// No need to check server logs separately
}
