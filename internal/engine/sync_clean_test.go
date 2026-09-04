package engine

import (
	"os"
	"strings"
	"testing"

	"easy42/internal/config"
	"easy42/internal/ssh"
)

func TestLiveSyncCleanUnusedInterface(t *testing.T) {
	host := "172.24.3.243"

	// Check if host is reachable
	pool := ssh.NewClientPool()
	defer pool.CloseAll()
	client, sftpClient, err := pool.GetClient(host)
	if err != nil {
		t.Skipf("Skipping live test: host %s unreachable: %v", host, err)
	}

	testIface := "wg42testclean"
	confPath := "/etc/wireguard/" + testIface + ".conf"

	// Create dummy interface and dummy conf file on remote device
	_, _ = ssh.RunCommand(client, "ip link del dev "+testIface+" 2>/dev/null")
	_, err = ssh.RunCommand(client, "ip link add dev "+testIface+" type wireguard")
	if err != nil {
		t.Fatalf("Failed to create dummy wg interface: %v", err)
	}
	defer func() {
		_, _ = ssh.RunCommand(client, "ip link del dev "+testIface+" 2>/dev/null")
		if sftpClient != nil {
			_ = sftpClient.Remove(confPath)
		}
	}()

	_ = ssh.AtomicWriteFile(sftpClient, confPath, []byte("[Interface]\nListenPort = 59999\n"), 0600)

	// Verify the interface is present in `wg`
	running, err := ssh.GetRunningWgInterfaces(client)
	if err != nil {
		t.Fatalf("GetRunningWgInterfaces failed: %v", err)
	}
	found := false
	for _, iface := range running {
		if iface == testIface {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Expected %s in running interfaces: %v", testIface, running)
	}

	// Setup manager with node243 and no link for testIface
	tempDir, err := os.MkdirTemp("", "easy42-live-clean-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := NewManager(store)
	if err := mgr.Unlock(pass); err != nil {
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	node := config.Node{
		Name:      "node243",
		Host:      host,
		IP:        "172.24.3.243",
		Interface: "ens3",
		ASN:       4224424309,
	}
	if err := mgr.AddNode(node); err != nil {
		t.Fatalf("Failed to add node: %v", err)
	}

	// Run PlanSync
	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}

	var deleteAction *config.SyncAction
	for i := range actions {
		if actions[i].Interface == testIface && actions[i].Type == config.ActionDeleteConfig {
			deleteAction = &actions[i]
			break
		}
	}
	if deleteAction == nil {
		t.Fatalf("PlanSync did not detect unused interface %s, planned actions: %+v", testIface, actions)
	}

	// Run ExecuteSync
	results, err := mgr.ExecuteSync()
	if err != nil {
		t.Fatalf("ExecuteSync failed: %v", err)
	}

	var cleanResult *config.SyncResult
	for i := range results {
		if strings.Contains(results[i].Action, testIface) {
			cleanResult = &results[i]
			break
		}
	}
	if cleanResult == nil || !cleanResult.Success {
		t.Fatalf("Expected successful clean result for %s, got: %+v", testIface, results)
	}

	// Verify interface is gone from `wg` and file is gone from /etc/wireguard/
	runningAfter, _ := ssh.GetRunningWgInterfaces(client)
	for _, iface := range runningAfter {
		if iface == testIface {
			t.Fatalf("Interface %s still exists in `wg` after sync!", testIface)
		}
	}

	exists := ssh.InterfaceExists(client, testIface)
	if exists {
		t.Fatalf("Interface %s still exists on system after sync!", testIface)
	}

	_, readErr := ssh.ReadRemoteFile(sftpClient, confPath)
	if readErr == nil {
		t.Fatalf("Conf file %s still exists after sync!", confPath)
	}
}

func TestLiveSyncStartsUnstartedInterface(t *testing.T) {
	host := "172.24.3.243"

	// Check if host is reachable
	pool := ssh.NewClientPool()
	defer pool.CloseAll()
	client, sftpClient, err := pool.GetClient(host)
	if err != nil {
		t.Skipf("Skipping live test: host %s unreachable: %v", host, err)
	}

	tempDir, err := os.MkdirTemp("", "easy42-live-start-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := NewManager(store)
	if err := mgr.Unlock(pass); err != nil {
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	nodeA := config.Node{
		Name:      "testnodea",
		Host:      host,
		IP:        "172.24.3.243",
		Interface: "ens3",
		ASN:       4224424309,
	}
	nodeB := config.Node{
		Name:      "testnodeb",
		Host:      host,
		IP:        "172.24.3.244",
		Interface: "ens3",
		ASN:       4224424310,
	}
	if err := mgr.AddNode(nodeA); err != nil {
		t.Fatalf("Failed to add nodeA: %v", err)
	}
	if err := mgr.AddNode(nodeB); err != nil {
		t.Fatalf("Failed to add nodeB: %v", err)
	}

	link, err := mgr.AddLink("testnodea", "testnodeb", 51888, 51889, nil)
	if err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}

	ifaceA := link.From.Interface
	defer func() {
		_ = ssh.CleanWireGuardInterface(client, sftpClient, ifaceA)
	}()

	// Ensure interface is cleaned down before sync
	_ = ssh.CleanWireGuardInterface(client, sftpClient, ifaceA)
	if ssh.IsInterfaceStarted(client, ifaceA) {
		t.Fatalf("Expected interface %s to not be started before sync", ifaceA)
	}

	// Execute sync - should push config and start wg42* interface
	results, err := mgr.ExecuteSync()
	if err != nil {
		t.Fatalf("ExecuteSync failed: %v", err)
	}

	foundResult := false
	for _, res := range results {
		if strings.Contains(res.Action, ifaceA) {
			foundResult = true
			if !res.Success {
				t.Fatalf("Expected action for %s to succeed, got error: %s", ifaceA, res.Error)
			}
		}
	}
	if !foundResult {
		t.Fatalf("Expected sync result for %s, got: %+v", ifaceA, results)
	}

	// Verify interface is now in started state
	if !ssh.IsInterfaceStarted(client, ifaceA) {
		t.Fatalf("Expected interface %s to be started after sync!", ifaceA)
	}
}
