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
		agentBin = filepath.Join("..", "..", "agent", "target", "debug", "easy42-agent")
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

	tmpHostsFile := filepath.Join(t.TempDir(), "hosts")
	initialHosts := "127.0.0.1 localhost\n# User custom entry\n10.0.0.99 myrouter\n"
	if err := os.WriteFile(tmpHostsFile, []byte(initialHosts), 0644); err != nil {
		t.Fatalf("failed to write initial hosts file: %v", err)
	}

	cmd := exec.CommandContext(ctx, agentBin,
		"--server", server.URL,
		"--token", testToken,
		"--node-name", nodeName,
		"--interval", "2",
	)
	cmd.Env = append(os.Environ(), "EASY42_HOSTS_PATH="+tmpHostsFile)
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

	// Test hosts file management via SyncHostsCmd
	hostsCmdResp, err := hub.SendCommand(cmdCtx, nodeName, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_SyncHosts{
			SyncHosts: &agentpb.SyncHostsCmd{
				Hosts: []*agentpb.HostRecord{
					{Name: "foo", Ip: "192.168.100.1"},
					{Name: "bar", Ip: "192.168.100.2"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SyncHosts command failed: %v", err)
	}
	if !hostsCmdResp.Success {
		t.Fatalf("expected SyncHosts command to succeed, got: %+v", hostsCmdResp)
	}

	// Verify /etc/hosts content preserved existing entries and sorted records
	hostsBytes, err := os.ReadFile(tmpHostsFile)
	if err != nil {
		t.Fatalf("failed to read updated hosts file: %v", err)
	}
	hostsContent := string(hostsBytes)
	if !strings.Contains(hostsContent, "127.0.0.1 localhost") || !strings.Contains(hostsContent, "10.0.0.99 myrouter") {
		t.Fatalf("existing hosts content was wiped: %s", hostsContent)
	}
	expectedBlock := "### EASY42_START\n192.168.100.2 bar bar.internal\n192.168.100.1 foo foo.internal\n### EASY42_END\n"
	if !strings.Contains(hostsContent, expectedBlock) {
		t.Fatalf("hosts content missing expected block:\n%s\nGot:\n%s", expectedBlock, hostsContent)
	}

	// Verify atomic temp file is not left behind
	tmpHostsPending := tmpHostsFile + "-easy42-tmp"
	if _, err := os.Stat(tmpHostsPending); !os.IsNotExist(err) {
		t.Fatalf("expected temporary hosts file to be cleaned up/renamed, but it exists: %s", tmpHostsPending)
	}

	// Test streaming SyncHosts AgentMessage push
	err = hub.SyncHosts(nodeName, []*agentpb.HostRecord{
		{Name: "foo", Ip: "192.168.100.1"},
		{Name: "bar", Ip: "192.168.100.2"},
		{Name: "zoo", Ip: "192.168.100.9"},
	})
	if err != nil {
		t.Fatalf("hub.SyncHosts failed: %v", err)
	}

	// Wait up to 3 seconds for async update to apply
	deadlineHosts := time.Now().Add(3 * time.Second)
	updatedWithZoo := false
	for time.Now().Before(deadlineHosts) {
		content, err := os.ReadFile(tmpHostsFile)
		if err == nil && strings.Contains(string(content), "192.168.100.9 zoo zoo.internal") {
			updatedWithZoo = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !updatedWithZoo {
		t.Fatalf("streaming hosts sync did not update hosts file in time")
	}
}

func TestRustAgentSyncActionAndHooks(t *testing.T) {
	agentBin := filepath.Join("..", "..", "bin", "easy42-agent")
	if _, err := os.Stat(agentBin); err != nil {
		agentBin = filepath.Join("..", "..", "agent", "target", "x86_64-unknown-linux-musl", "release", "easy42-agent")
	}
	if _, err := os.Stat(agentBin); err != nil {
		agentBin = filepath.Join("..", "..", "agent", "target", "debug", "easy42-agent")
	}
	if _, err := os.Stat(agentBin); err != nil {
		t.Skipf("agent binary not found at %s, skipping e2e test", agentBin)
	}

	hub := agent.NewHub()
	testToken := "e42_hooks_token_99999"
	nodeName := "e2e-hooks-node"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/ws" {
			http.NotFound(w, r)
			return
		}
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer "+testToken {
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

	tmpDir := t.TempDir()
	preFlag := filepath.Join(tmpDir, "pre.txt")
	postFlag := filepath.Join(tmpDir, "post.txt")
	configFile := filepath.Join(tmpDir, "agent.json")
	targetFile := filepath.Join(tmpDir, "sync_test.conf")

	agentCfgContent := `{
		"server": "` + server.URL + `",
		"token": "` + testToken + `",
		"node_name": "` + nodeName + `",
		"pre_sync": "echo $EASY42_SYNC_NODE:$EASY42_SYNC_ACTION:$EASY42_SYNC_TARGET_FILE:$EASY42_SYNC_PHASE > ` + preFlag + `",
		"post_sync": "echo $EASY42_SYNC_NODE:$EASY42_SYNC_SUCCESS:$EASY42_SYNC_STATUS:$EASY42_SYNC_PHASE > ` + postFlag + `"
	}`
	if err := os.WriteFile(configFile, []byte(agentCfgContent), 0600); err != nil {
		t.Fatalf("failed to write agent config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, agentBin, "--config", configFile)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start agent: %v", err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	// Wait for agent to connect
	deadline := time.Now().Add(5 * time.Second)
	connected := false
	for time.Now().Before(deadline) {
		if hub.IsConnected(nodeName) {
			connected = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !connected {
		t.Fatalf("agent did not connect within deadline")
	}

	cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cmdCancel()

	// Send SyncActionCmd
	cmdResp, err := hub.SendCommand(cmdCtx, nodeName, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_SyncAction{
			SyncAction: &agentpb.SyncActionCmd{
				ActionType:    "sync_roa",
				InterfaceName: "testiface",
				TargetFile:    targetFile,
				FileContent:   []byte("roa table test;"),
				FileMode:      0644,
				Description:   "Test sync roa",
				DiffStatus:    "create",
			},
		},
	})
	if err != nil {
		t.Fatalf("SyncAction failed: %v", err)
	}
	if !cmdResp.Success {
		t.Fatalf("expected SyncAction to succeed, got: %+v", cmdResp)
	}

	// Verify target file was written
	written, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("target file not found: %v", err)
	}
	if string(written) != "roa table test;" {
		t.Fatalf("unexpected content in target file: %s", string(written))
	}

	// Verify pre_sync hook output and env vars
	preData, err := os.ReadFile(preFlag)
	if err != nil {
		t.Fatalf("pre hook flag not found: %v", err)
	}
	expectedPre := nodeName + ":sync_roa:" + targetFile + ":pre"
	if strings.TrimSpace(string(preData)) != expectedPre {
		t.Fatalf("unexpected pre hook output: %s, expected: %s", string(preData), expectedPre)
	}

	// Verify post_sync hook output and env vars
	postData, err := os.ReadFile(postFlag)
	if err != nil {
		t.Fatalf("post hook flag not found: %v", err)
	}
	expectedPost := nodeName + ":true:success:post"
	if strings.TrimSpace(string(postData)) != expectedPost {
		t.Fatalf("unexpected post hook output: %s, expected: %s", string(postData), expectedPost)
	}
}

func TestRustAgentSyncActionPreHookFailure(t *testing.T) {
	agentBin := filepath.Join("..", "..", "bin", "easy42-agent")
	if _, err := os.Stat(agentBin); err != nil {
		agentBin = filepath.Join("..", "..", "agent", "target", "debug", "easy42-agent")
	}
	if _, err := os.Stat(agentBin); err != nil {
		t.Skipf("agent binary not found at %s, skipping e2e test", agentBin)
	}

	hub := agent.NewHub()
	testToken := "e42_hooks_fail_token_111"
	nodeName := "e2e-hooks-fail-node"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/ws" {
			http.NotFound(w, r)
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

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "agent.json")
	targetFile := filepath.Join(tmpDir, "blocked_test.conf")

	agentCfgContent := `{
		"server": "` + server.URL + `",
		"token": "` + testToken + `",
		"node_name": "` + nodeName + `",
		"pre_sync": "echo 'custom pre failure' >&2; exit 19"
	}`
	if err := os.WriteFile(configFile, []byte(agentCfgContent), 0600); err != nil {
		t.Fatalf("failed to write agent config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, agentBin, "--config", configFile)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start agent: %v", err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	deadline := time.Now().Add(5 * time.Second)
	connected := false
	for time.Now().Before(deadline) {
		if hub.IsConnected(nodeName) {
			connected = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !connected {
		t.Fatalf("agent did not connect within deadline")
	}

	cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cmdCancel()

	cmdResp, err := hub.SendCommand(cmdCtx, nodeName, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_SyncAction{
			SyncAction: &agentpb.SyncActionCmd{
				ActionType:    "sync_roa",
				InterfaceName: "testiface",
				TargetFile:    targetFile,
				FileContent:   []byte("should never be written"),
				FileMode:      0644,
				Description:   "Test blocked sync roa",
				DiffStatus:    "create",
			},
		},
	})
	if err != nil {
		t.Fatalf("SendCommand error: %v", err)
	}
	if cmdResp.Success {
		t.Fatalf("expected command to fail due to pre_sync exit code 19, but it succeeded")
	}
	if cmdResp.ExitCode != 19 {
		t.Fatalf("expected exit code 19, got %d", cmdResp.ExitCode)
	}
	if !strings.Contains(cmdResp.Stderr, "pre_sync hook failed") || !strings.Contains(cmdResp.Stderr, "custom pre failure") {
		t.Fatalf("expected pre_sync hook error message in stderr, got: %s", cmdResp.Stderr)
	}

	// File should NOT exist
	if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
		t.Fatalf("target file should NOT have been created when pre_sync fails")
	}
}


