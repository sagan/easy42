package agent_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
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
	"easy42/internal/config"
)

// sshKeyBlob builds a well-formed SSH public key blob. The public fields are
// filled from payload, so distinct payloads give distinct keys while the blob
// stays exactly one valid public key.
func sshKeyBlob(algorithm, payload string) string {
	appendField := func(buf []byte, field []byte) []byte {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(field)))
		buf = append(buf, l[:]...)
		return append(buf, field...)
	}
	derive := func(n int) []byte {
		out := make([]byte, n)
		for i := range out {
			out[i] = payload[i%len(payload)]
		}
		return out
	}
	buf := appendField(nil, []byte(algorithm))
	if algorithm == "ssh-rsa" {
		buf = appendField(buf, []byte{0x01, 0x00, 0x01})
		buf = appendField(buf, append([]byte{0x01}, derive(32)...))
	} else {
		buf = appendField(buf, derive(32))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// dropbearEd25519Record builds a native Dropbear Ed25519 key file: the key type
// followed by one string holding `private scalar || public point`. Only the
// 32-byte point is the public key. Returns the record and the base64 blob the
// agent must publish (`algorithm || string(point)`).
func dropbearEd25519Record(scalar, point []byte) (record []byte, publicKey string) {
	record = []byte{0, 0, 0, 11}
	record = append(record, "ssh-ed25519"...)
	record = append(record, 0, 0, 0, 64)
	record = append(record, scalar...)
	record = append(record, point...)

	pub := []byte{0, 0, 0, 11}
	pub = append(pub, "ssh-ed25519"...)
	pub = append(pub, 0, 0, 0, 32)
	pub = append(pub, point...)
	return record, base64.StdEncoding.EncodeToString(pub)
}

type runningAgent struct {
	name        string
	sshDir      string
	hostKeyDir  string
	dropbearDir string
	cancel      context.CancelFunc
}

func findAgentBinary(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "..", "bin", "easy42-agent"),
		filepath.Join("..", "..", "agent", "target", "x86_64-unknown-linux-musl", "release", "easy42-agent"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skipf("agent binary not found, skipping e2e test")
	return ""
}

// startAgent runs the real easy42-agent binary with an isolated SSH directory.
func startAgent(t *testing.T, agentBin, serverURL, token, name string, extraArgs ...string) *runningAgent {
	t.Helper()
	sshDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	args := append([]string{
		"--server", serverURL,
		"--token", token,
		"--node-name", name,
		"--interval", "1",
		"--config", filepath.Join(t.TempDir(), "agent.json"),
	}, extraArgs...)
	cmd := exec.CommandContext(ctx, agentBin, args...)
	hostKeyDir := t.TempDir()
	dropbearDir := t.TempDir()
	cmd.Env = append(os.Environ(),
		"EASY42_SSH_DIR="+sshDir,
		"EASY42_SSH_HOST_KEY_DIR="+hostKeyDir+":"+dropbearDir,
		"EASY42_HOSTS_PATH="+filepath.Join(t.TempDir(), "hosts"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("failed to start agent %s: %v", name, err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	return &runningAgent{name: name, sshDir: sshDir, hostKeyDir: hostKeyDir, dropbearDir: dropbearDir, cancel: cancel}
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestRustAgentSSHKeysEndToEnd(t *testing.T) {
	agentBin := findAgentBinary(t)

	hub := agent.NewHub()
	tokenA := "e42_e2e_ssh_a"
	tokenB := "e42_e2e_ssh_b"
	nodeA := "e2e-ssh-a"
	nodeB := "e2e-ssh-b"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/ws" {
			http.NotFound(w, r)
			return
		}
		var name string
		switch r.Header.Get("Authorization") {
		case "Bearer " + tokenA:
			name = nodeA
		case "Bearer " + tokenB:
			name = nodeB
		default:
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := agent.NewConnection(name, ws, hub)
		hub.Register(conn)
		go conn.ReadLoop()
	}))
	defer server.Close()

	// Node A has two default identity keys plus a non-default key that must be
	// ignored by the collector.
	runA := startAgent(t, agentBin, server.URL, tokenA, nodeA)
	keyA := sshKeyBlob("ssh-ed25519", "node-a-key")
	if err := os.WriteFile(filepath.Join(runA.sshDir, "id_ed25519.pub"),
		[]byte("ssh-ed25519 "+keyA+" root@node-a\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rsaA := sshKeyBlob("ssh-rsa", "node-a-rsa")
	if err := os.WriteFile(filepath.Join(runA.sshDir, "id_rsa.pub"),
		[]byte("ssh-rsa "+rsaA+" root@node-a-rsa\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runA.sshDir, "custom.pub"),
		[]byte("ssh-ed25519 "+sshKeyBlob("ssh-ed25519", "ignored")+" custom\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Node A also publishes its SSH *server* host key. It is a different key from
	// the login identities above and must be tagged with kind "host".
	hostKeyA := sshKeyBlob("ssh-ed25519", "node-a-host-key")
	if err := os.WriteFile(filepath.Join(runA.hostKeyDir, "ssh_host_ed25519_key.pub"),
		[]byte("ssh-ed25519 "+hostKeyA+" root@node-a\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runB := startAgent(t, agentBin, server.URL, tokenB, nodeB)
	keyB := sshKeyBlob("ssh-ed25519", "node-b-key")
	if err := os.WriteFile(filepath.Join(runB.sshDir, "id_ed25519.pub"),
		[]byte("ssh-ed25519 "+keyB+" root@node-b\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Node B runs Dropbear (OpenWrt): its host key lives in a single binary file
	// with no `.pub` companion and must still be collected. The record holds the
	// private scalar and the public point, and only the point may be published.
	dropbearFile, hostKeyB := dropbearEd25519Record(
		bytes.Repeat([]byte{0x11}, 32),
		bytes.Repeat([]byte{0x42}, 32),
	)
	if err := os.WriteFile(filepath.Join(runB.hostKeyDir, "dropbear_ed25519_host_key"),
		dropbearFile, 0600); err != nil {
		t.Fatal(err)
	}

	waitFor(t, "both agents to connect", 10*time.Second, func() bool {
		return hub.IsConnected(nodeA) && hub.IsConnected(nodeB)
	})

	// 1. Keys are collected automatically via telemetry: two login identities and
	// one SSH server host key for node A.
	waitFor(t, "node A keys to be collected", 10*time.Second, func() bool {
		keys := hub.GetSSHKeys(nodeA)
		host, user := 0, 0
		for _, k := range keys {
			if k.Kind == config.SSHKeyKindHost {
				host++
			} else {
				user++
			}
		}
		return len(keys) == 3 && host == 1 && user == 2
	})
	// Node B: one login identity plus one Dropbear host key.
	var dropbearKey config.SSHPublicKey
	waitFor(t, "node B keys to be collected", 10*time.Second, func() bool {
		keys := hub.GetSSHKeys(nodeB)
		host, user := 0, 0
		for _, k := range keys {
			if k.Kind == config.SSHKeyKindHost {
				host++
				dropbearKey = k
			} else {
				user++
			}
		}
		return len(keys) == 2 && host == 1 && user == 1
	})
	if dropbearKey.Key != hostKeyB {
		t.Fatalf("Dropbear host key must be extracted from dropbear_ed25519_host_key, got %+v", dropbearKey)
	}
	// The private scalar must not leak into the published key, and the key must
	// be a valid, exact public key (otherwise known_hosts never matches).
	if err := config.ValidateSSHPublicKey(dropbearKey.Algorithm, dropbearKey.Key); err != nil {
		t.Fatalf("published Dropbear key is not an exact public key: %v", err)
	}
	if !strings.Contains(dropbearKey.Path, "dropbear_ed25519_host_key") {
		t.Fatalf("unexpected Dropbear host key path %q", dropbearKey.Path)
	}
	for _, k := range hub.GetSSHKeys(nodeA) {
		if k.Comment == "custom" {
			t.Fatalf("non-default identity files must not be collected: %+v", k)
		}
		if k.Kind == config.SSHKeyKindHost {
			if k.Key != hostKeyA {
				t.Fatalf("unexpected host key collected: %+v", k)
			}
			if !strings.Contains(k.Path, "ssh_host_ed25519_key.pub") {
				t.Fatalf("host key path must point at /etc/ssh/ssh_host_*_key.pub, got %q", k.Path)
			}
		}
	}

	// 2. The explicit fetch command returns the keys and the current block hashes.
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fetchCancel()
	fetchResp, err := hub.SendCommand(fetchCtx, nodeA, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_FetchSshKeys{
			FetchSshKeys: &agentpb.FetchSshKeysCmd{},
		},
	})
	if err != nil || !fetchResp.Success {
		t.Fatalf("fetch SSH keys failed: %v %v", err, fetchResp)
	}
	var fetched struct {
		SSHKeys            []struct{ Algorithm, Key, Comment, Path string } `json:"ssh_keys"`
		SSHKeysEnabled     bool                                             `json:"ssh_keys_enabled"`
		KnownHostsHash     string                                           `json:"known_hosts_hash"`
		AuthorizedKeysHash string                                           `json:"authorized_keys_hash"`
	}
	if err := json.Unmarshal([]byte(fetchResp.Stdout), &fetched); err != nil {
		t.Fatalf("invalid fetch response: %v (%s)", err, fetchResp.Stdout)
	}
	if !fetched.SSHKeysEnabled || len(fetched.SSHKeys) != 3 {
		t.Fatalf("unexpected fetch result: %+v", fetched)
	}
	if fetched.KnownHostsHash != "" || fetched.AuthorizedKeysHash != "" {
		t.Fatalf("no managed block exists yet, expected empty hashes: %+v", fetched)
	}

	// 3. Apply an authorized_keys sync action directly through the agent and
	// verify it is idempotent, atomic and preserves foreign content.
	foreign := "ssh-ed25519 " + sshKeyBlob("ssh-ed25519", "manual") + " manually-added\n"
	if err := os.WriteFile(filepath.Join(runB.sshDir, "authorized_keys"), []byte(foreign), 0600); err != nil {
		t.Fatal(err)
	}

	sendSync := func() *agentpb.CommandResponse {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := hub.SendCommand(ctx, nodeB, &agentpb.CommandRequest{
			Command: &agentpb.CommandRequest_SyncAction{
				SyncAction: &agentpb.SyncActionCmd{
					ActionType: "sync_ssh_keys",
					TargetFile: config.SSHAuthorizedKeysPath,
					FileMode:   0644,
					DiffStatus: "create",
					SshKeys: []*agentpb.SshPublicKey{
						{Algorithm: "ssh-ed25519", Key: keyA, Comment: "root@node-a"},
						{Algorithm: "ssh-rsa", Key: rsaA, Comment: "root@node-a-rsa"},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("SendCommand failed: %v", err)
		}
		return resp
	}

	resp := sendSync()
	if !resp.Success {
		t.Fatalf("authorized_keys sync failed: %s %s", resp.Stderr, resp.Stdout)
	}
	if !strings.Contains(resp.Stdout, "modified=true") {
		t.Fatalf("expected first sync to modify the file, got %q", resp.Stdout)
	}

	content, err := os.ReadFile(filepath.Join(runB.sshDir, "authorized_keys"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "manually-added") {
		t.Fatalf("foreign authorized_keys lines must be preserved, got:\n%s", text)
	}
	if !strings.Contains(text, "ssh-ed25519 "+keyA+" easy42 root@node-a\n") {
		t.Fatalf("expected managed ed25519 line, got:\n%s", text)
	}
	if !strings.Contains(text, "ssh-rsa "+rsaA+" easy42 root@node-a-rsa\n") {
		t.Fatalf("expected managed rsa line, got:\n%s", text)
	}
	if !strings.HasPrefix(text, foreign) {
		t.Fatalf("foreign content must stay at the top of the file, got:\n%s", text)
	}
	info, err := os.Stat(filepath.Join(runB.sshDir, "authorized_keys"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("authorized_keys must be mode 0600, got %o", info.Mode().Perm())
	}

	resp = sendSync()
	if !resp.Success || !strings.Contains(resp.Stdout, "modified=false") {
		t.Fatalf("second sync must be a no-op, got success=%v stdout=%q", resp.Success, resp.Stdout)
	}

	// 4. known_hosts is managed the same way with mode 0644.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err = hub.SendCommand(ctx, nodeB, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_SyncAction{
			SyncAction: &agentpb.SyncActionCmd{
				ActionType: "sync_ssh_keys",
				TargetFile: config.SSHKnownHostsPath,
				FileMode:   0644,
				DiffStatus: "create",
				SshKeys: []*agentpb.SshPublicKey{
					{Algorithm: "ssh-ed25519", Key: keyA, Comment: "root@node-a", Kind: config.SSHKeyKindHost},
				},
			},
		},
	})
	if err != nil || !resp.Success {
		t.Fatalf("known_hosts sync failed: %v %v", err, resp)
	}
	khInfo, err := os.Stat(filepath.Join(runB.sshDir, "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	if khInfo.Mode().Perm() != 0644 {
		t.Fatalf("known_hosts must be mode 0644, got %o", khInfo.Mode().Perm())
	}

	// 4b. The agent writes the file content it receives verbatim (newlines and
	// all); it must never re-flow the block into a single line.
	khBlock := "### EASY42_START\n" +
		"10.0.0.1,node-a,node-a.internal ssh-ed25519 " + keyA + " root@node-a\n" +
		"### EASY42_END\n"
	resp, err = hub.SendCommand(ctx, nodeB, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_SyncAction{
			SyncAction: &agentpb.SyncActionCmd{
				ActionType:  "sync_ssh_keys",
				TargetFile:  config.SSHKnownHostsPath,
				FileMode:    0644,
				DiffStatus:  "update",
				FileContent: []byte(khBlock),
			},
		},
	})
	if err != nil || !resp.Success {
		t.Fatalf("known_hosts content sync failed: %v %v", err, resp)
	}
	kh, err := os.ReadFile(filepath.Join(runB.sshDir, "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(kh); got != khBlock {
		t.Fatalf("known_hosts content must be written verbatim, got:\n%q\nwant:\n%q", got, khBlock)
	}

	// 5. An empty key set removes the block but keeps foreign lines.
	resp, err = hub.SendCommand(ctx, nodeB, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_SyncAction{
			SyncAction: &agentpb.SyncActionCmd{
				ActionType: "sync_ssh_keys",
				TargetFile: config.SSHAuthorizedKeysPath,
				FileMode:   0644,
				DiffStatus: "create",
			},
		},
	})
	if err != nil || !resp.Success {
		t.Fatalf("authorized_keys cleanup failed: %v %v", err, resp)
	}
	content, _ = os.ReadFile(filepath.Join(runB.sshDir, "authorized_keys"))
	text = string(content)
	if strings.Contains(text, config.SSHKeysStartMarker) {
		t.Fatalf("empty key set must remove the managed block, got:\n%s", text)
	}
	if !strings.Contains(text, "manually-added") {
		t.Fatalf("cleanup must keep foreign lines, got:\n%s", text)
	}

	// 6. Unmanaged targets are rejected.
	resp, err = hub.SendCommand(ctx, nodeB, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_SyncAction{
			SyncAction: &agentpb.SyncActionCmd{
				ActionType: "sync_ssh_keys",
				TargetFile: "/root/.ssh/id_ed25519",
				DiffStatus: "create",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Success {
		t.Fatal("expected unmanaged target paths to be rejected")
	}
}

func TestRustAgentSSHKeysDisabledViaFlag(t *testing.T) {
	agentBin := findAgentBinary(t)

	hub := agent.NewHub()
	token := "e42_e2e_ssh_disabled"
	node := "e2e-ssh-disabled"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/ws" || r.Header.Get("Authorization") != "Bearer "+token {
			http.NotFound(w, r)
			return
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := agent.NewConnection(node, ws, hub)
		hub.Register(conn)
		go conn.ReadLoop()
	}))
	defer server.Close()

	run := startAgent(t, agentBin, server.URL, token, node, "--disable-apps")
	key := sshKeyBlob("ssh-ed25519", "disabled-key")
	if err := os.WriteFile(filepath.Join(run.sshDir, "id_ed25519.pub"),
		[]byte("ssh-ed25519 "+key+" root@disabled\n"), 0600); err != nil {
		t.Fatal(err)
	}

	waitFor(t, "agent to connect", 10*time.Second, func() bool { return hub.IsConnected(node) })
	waitFor(t, "agent to report disable_apps", 10*time.Second, func() bool { return hub.IsAppsDisabled(node) })

	// Give telemetry a couple of cycles; keys must never be published.
	time.Sleep(2500 * time.Millisecond)
	if keys := hub.GetSSHKeys(node); len(keys) != 0 {
		t.Fatalf("keys must not be collected with disable_apps = true, got %+v", keys)
	}

	// A sync action is refused as well.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := hub.SendCommand(ctx, node, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_SyncAction{
			SyncAction: &agentpb.SyncActionCmd{
				ActionType: "sync_ssh_keys",
				TargetFile: config.SSHAuthorizedKeysPath,
				DiffStatus: "create",
				SshKeys:    []*agentpb.SshPublicKey{{Algorithm: "ssh-ed25519", Key: key}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Success || !strings.Contains(resp.Stderr, "disabled") {
		t.Fatalf("expected the sync to be refused, got success=%v stderr=%q", resp.Success, resp.Stderr)
	}
	if _, err := os.Stat(filepath.Join(run.sshDir, "authorized_keys")); err == nil {
		t.Fatal("authorized_keys must not be written on a disable_apps node")
	}
}
