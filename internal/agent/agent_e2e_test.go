package agent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestRustAgentEndToEnd(t *testing.T) {
	agentBin := filepath.Join("..", "..", "bin", "easy42-agent")
	if _, err := os.Stat(agentBin); err != nil {
		agentBin = filepath.Join("..", "..", "agent", "target", "x86_64-unknown-linux-musl", "release", "easy42-agent")
	}
	if _, err := os.Stat(agentBin); err != nil {
		t.Skipf("agent binary not found at %s, skipping e2e test", agentBin)
	}

	hub := agent.NewHub()
	testToken := "e42_e2e_test_token_12345"
	nodeName := "e2e-node-1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/ws" {
			http.NotFound(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + testToken
		if authHeader != expectedAuth {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		conn := agent.NewConnection(nodeName, ws, hub)
		hub.Register(conn)
		go conn.ReadLoop()
	}))
	defer server.Close()

	// Launch easy42-agent binary
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, agentBin,
		"--server", server.URL,
		"--token", testToken,
		"--node-name", nodeName,
		"--interval", "2",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start easy42-agent: %v", err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	// Wait for agent to connect and register
	deadline := time.Now().Add(5 * time.Second)
	connected := false
	for time.Now().Before(deadline) {
		if hub.IsConnected(nodeName) {
			connected = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !connected {
		t.Fatalf("agent did not connect to hub within deadline")
	}

	// Wait for telemetry to arrive
	time.Sleep(500 * time.Millisecond)
	status := hub.NodeStatus(nodeName)
	if !status.Connected {
		t.Fatalf("expected node status to be connected")
	}

	// Test sending CommandRequest: ApplyConfigFile
	tmpTestFile := filepath.Join(t.TempDir(), "test_file.txt")
	testContent := "hello easy42 from rust agent e2e"

	cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cmdCancel()

	cmdResp, err := hub.SendCommand(cmdCtx, nodeName, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_ApplyConfig{
			ApplyConfig: &agentpb.ApplyConfigFile{
				Path:     tmpTestFile,
				Content:  []byte(testContent),
				FileMode: 0644,
			},
		},
	})
	if err != nil {
		t.Fatalf("SendCommand failed: %v", err)
	}
	if !cmdResp.Success {
		t.Fatalf("expected ApplyConfigFile to succeed, got: %+v", cmdResp)
	}

	// Verify file was written to disk by the Rust agent with 0644 permissions
	stat, err := os.Stat(tmpTestFile)
	if err != nil {
		t.Fatalf("failed to stat written file: %v", err)
	}
	if stat.Mode().Perm() != 0644 {
		t.Fatalf("expected file perm 0644, got %o", stat.Mode().Perm())
	}
	writtenBytes, err := os.ReadFile(tmpTestFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(writtenBytes) != testContent {
		t.Fatalf("file content mismatch: expected %q, got %q", testContent, string(writtenBytes))
	}

	// Test writing .nft file in nested directory without explicit mode -> should get 0755 and dir 0755
	tmpSubDir := filepath.Join(t.TempDir(), "nftdir")
	tmpNftFile := filepath.Join(tmpSubDir, "rule.nft")
	nftResp, err := hub.SendCommand(cmdCtx, nodeName, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_ApplyConfig{
			ApplyConfig: &agentpb.ApplyConfigFile{
				Path:     tmpNftFile,
				Content:  []byte("#!/usr/sbin/nft -f\nflush ruleset\n"),
				FileMode: 0, // Unset -> should auto-derive 0755
			},
		},
	})
	if err != nil || !nftResp.Success {
		t.Fatalf("ApplyConfigFile for nft failed: err=%v resp=%+v", err, nftResp)
	}
	nftStat, err := os.Stat(tmpNftFile)
	if err != nil {
		t.Fatalf("failed to stat nft file: %v", err)
	}
	if nftStat.Mode().Perm() != 0755 {
		t.Fatalf("expected nft perm 0755, got %o", nftStat.Mode().Perm())
	}
	dirStat, err := os.Stat(tmpSubDir)
	if err != nil {
		t.Fatalf("failed to stat nft dir: %v", err)
	}
	if dirStat.Mode().Perm() != 0755 {
		t.Fatalf("expected dir perm 0755, got %o", dirStat.Mode().Perm())
	}

	// Test diagnostic command
	diagResp, err := hub.SendCommand(cmdCtx, nodeName, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_LookingGlass{
			LookingGlass: &agentpb.LookingGlassCmd{
				Tool:           agentpb.LookingGlassCmd_CUSTOM,
				Command:        "ip route show",
				TimeoutSeconds: 5,
			},
		},
	})
	if err != nil {
		t.Fatalf("LookingGlass command failed: %v", err)
	}
	if !diagResp.Success {
		t.Fatalf("expected LookingGlass to succeed, got: %+v", diagResp)
	}
	if strings.TrimSpace(diagResp.Stdout) == "" && strings.TrimSpace(diagResp.Stderr) == "" {
		t.Fatalf("expected non-empty output from ip route show")
	}
}
