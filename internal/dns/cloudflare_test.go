package dns

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"easy42/internal/config"
)

type mockCFServer struct {
	mu      sync.Mutex
	records map[string]DNSRecord
	counter int
}

func newMockCFServer() (*mockCFServer, *httptest.Server) {
	m := &mockCFServer{
		records: make(map[string]DNSRecord),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"errors":  []map[string]any{{"code": 10000, "message": "Authentication error"}},
			})
			return
		}

		path := r.URL.Path
		// GET /zones/{zone_id}
		if r.Method == http.MethodGet && strings.HasPrefix(path, "/zones/") && !strings.Contains(strings.TrimPrefix(path, "/zones/"), "/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": map[string]any{
					"id":   strings.TrimPrefix(path, "/zones/"),
					"name": "example.com",
				},
			})
			return
		}

		// /zones/{zone_id}/dns_records
		if strings.HasSuffix(path, "/dns_records") {
			switch r.Method {
			case http.MethodGet:
				nameFilter := r.URL.Query().Get("name")
				var list []DNSRecord
				for _, rec := range m.records {
					if nameFilter == "" || strings.EqualFold(rec.Name, nameFilter) {
						list = append(list, rec)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"result":  list,
					"result_info": map[string]any{
						"page":        1,
						"per_page":    100,
						"count":       len(list),
						"total_count": len(list),
						"total_pages": 1,
					},
				})
				return

			case http.MethodPost:
				var rec DNSRecord
				if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				m.counter++
				rec.ID = "rec-" + string(rune('0'+m.counter))
				m.records[rec.ID] = rec

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"result":  rec,
				})
				return
			}
		}

		// /zones/{zone_id}/dns_records/{record_id}
		parts := strings.Split(path, "/")
		if len(parts) >= 5 && parts[3] == "dns_records" {
			id := parts[4]
			switch r.Method {
			case http.MethodPut:
				var rec DNSRecord
				if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				rec.ID = id
				m.records[id] = rec
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"result":  rec,
				})
				return

			case http.MethodDelete:
				delete(m.records, id)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"result":  map[string]any{"id": id},
				})
				return
			}
		}

		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewServer(handler)
	return m, server
}

func TestExpectedRecordsForNode(t *testing.T) {
	node := config.Node{
		Name: "node1",
		IP:   "192.168.100.1",
		IP6:  "fd42:a159:f9f0::1/48",
	}

	// Toggle OFF
	recs1 := ExpectedRecordsForNode(node, "easy42.example.com", false)
	if len(recs1) != 4 {
		t.Fatalf("expected 4 records when toggle false, got %d: %+v", len(recs1), recs1)
	}
	findRec := func(recs []DNSRecord, rType, name string) *DNSRecord {
		for _, r := range recs {
			if r.Type == rType && strings.EqualFold(r.Name, name) {
				return &r
			}
		}
		return nil
	}

	aRec := findRec(recs1, "A", "node1.easy42.example.com")
	if aRec == nil || aRec.Content != "192.168.100.1" {
		t.Fatalf("unexpected A record: %+v", aRec)
	}
	wildARec := findRec(recs1, "A", "*.node1.easy42.example.com")
	if wildARec == nil || wildARec.Content != "192.168.100.1" {
		t.Fatalf("unexpected wildcard A record: %+v", wildARec)
	}
	aaaaRec := findRec(recs1, "AAAA", "node1.easy42.example.com")
	if aaaaRec == nil || aaaaRec.Content != "fd42:a159:f9f0::1" {
		t.Fatalf("unexpected AAAA record: %+v", aaaaRec)
	}
	wildAAAARec := findRec(recs1, "AAAA", "*.node1.easy42.example.com")
	if wildAAAARec == nil || wildAAAARec.Content != "fd42:a159:f9f0::1" {
		t.Fatalf("unexpected wildcard AAAA record: %+v", wildAAAARec)
	}

	// Toggle ON
	recs2 := ExpectedRecordsForNode(node, "easy42.example.com", true)
	if len(recs2) != 3 {
		t.Fatalf("expected 3 records when toggle true, got %d: %+v", len(recs2), recs2)
	}
	aRec2 := findRec(recs2, "A", "node1.easy42.example.com")
	if aRec2 == nil || aRec2.Content != "192.168.100.1" {
		t.Fatalf("unexpected A record: %+v", aRec2)
	}
	wildARec2 := findRec(recs2, "A", "*.node1.easy42.example.com")
	if wildARec2 == nil || wildARec2.Content != "192.168.100.1" {
		t.Fatalf("unexpected wildcard A record: %+v", wildARec2)
	}
	aaaaRec2 := findRec(recs2, "AAAA", "node16.easy42.example.com")
	if aaaaRec2 == nil || aaaaRec2.Content != "fd42:a159:f9f0::1" {
		t.Fatalf("unexpected AAAA record when toggle true: %+v", aaaaRec2)
	}
}

func TestSync(t *testing.T) {
	mock, srv := newMockCFServer()
	defer srv.Close()

	client := NewClient("zone-123", "secret-token", "easy42.example.com").WithBaseURL(srv.URL)

	nodes := []config.Node{
		{Name: "n1", IP: "10.0.0.1", IP6: "fd42::1"},
		{Name: "n2", IP: "10.0.0.2", IP6: "fd42::2"},
	}

	dnsCfg := config.DNSConfig{
		ZoneID:             "zone-123",
		APIToken:           "secret-token",
		BaseDomain:         "easy42.example.com",
		PublishIPv6OwnName: false,
	}

	// 1. Initial sync (creates all 8 records: 2 nodes * 4 records)
	res, err := Sync(context.Background(), client, nodes, dnsCfg, false)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if res.Created != 8 {
		t.Errorf("expected 8 created, got %d", res.Created)
	}
	if len(mock.records) != 8 {
		t.Errorf("expected 8 records in mock, got %d", len(mock.records))
	}

	// 2. Second sync with no changes (should ignore all 8)
	res2, err := Sync(context.Background(), client, nodes, dnsCfg, false)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if res2.Ignored != 8 {
		t.Errorf("expected 8 ignored, got %d", res2.Ignored)
	}

	// 3. Update an IP (updates both n1 and *.n1 A records)
	nodes[0].IP = "10.0.0.10"
	res3, err := Sync(context.Background(), client, nodes, dnsCfg, false)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if res3.Updated != 2 || res3.Ignored != 6 {
		t.Errorf("expected 2 updated, 6 ignored, got updated=%d, ignored=%d", res3.Updated, res3.Ignored)
	}

	// 4. Toggle PublishIPv6OwnName = true
	dnsCfg.PublishIPv6OwnName = true
	res4, err := Sync(context.Background(), client, nodes, dnsCfg, false)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	// Should create 2 new AAAA at n16 and n26, ignore 4 A records, and delete old 4 AAAA at n1, *.n1, n2, *.n2
	if res4.Created != 2 || res4.Deleted != 4 || res4.Ignored != 4 {
		t.Errorf("expected 2 created, 4 deleted, 4 ignored, got created=%d, deleted=%d, ignored=%d",
			res4.Created, res4.Deleted, res4.Ignored)
	}
}

func TestForceSyncDeletesOrphanedRecords(t *testing.T) {
	mock, srv := newMockCFServer()
	defer srv.Close()

	client := NewClient("zone-123", "secret-token", "easy42.example.com").WithBaseURL(srv.URL)

	// Add orphaned records to mock that don't correspond to any node
	mock.records["orphan-1"] = DNSRecord{
		ID:      "orphan-1",
		Type:    "A",
		Name:    "ghost.easy42.example.com",
		Content: "10.99.99.99",
	}
	mock.records["orphan-wild"] = DNSRecord{
		ID:      "orphan-wild",
		Type:    "A",
		Name:    "*.ghost.easy42.example.com",
		Content: "10.99.99.99",
	}
	mock.records["orphan-2"] = DNSRecord{
		ID:      "orphan-2",
		Type:    "AAAA",
		Name:    "ghost6.easy42.example.com",
		Content: "fd42::9999",
	}
	// Record outside easy42.example.com should NOT be deleted
	mock.records["root-domain"] = DNSRecord{
		ID:      "root-domain",
		Type:    "A",
		Name:    "example.com",
		Content: "1.2.3.4",
	}

	nodes := []config.Node{
		{Name: "active1", IP: "10.0.0.1", IP6: "fd42::1"},
	}

	dnsCfg := config.DNSConfig{
		ZoneID:             "zone-123",
		APIToken:           "secret-token",
		BaseDomain:         "easy42.example.com",
		PublishIPv6OwnName: true,
	}

	res, err := Sync(context.Background(), client, nodes, dnsCfg, true)
	if err != nil {
		t.Fatalf("Force Sync failed: %v", err)
	}

	if res.Deleted != 3 {
		t.Errorf("expected 3 orphans deleted, got %d", res.Deleted)
	}

	if _, ok := mock.records["orphan-1"]; ok {
		t.Errorf("orphan-1 was not deleted")
	}
	if _, ok := mock.records["orphan-wild"]; ok {
		t.Errorf("orphan-wild was not deleted")
	}
	if _, ok := mock.records["orphan-2"]; ok {
		t.Errorf("orphan-2 was not deleted")
	}
	if _, ok := mock.records["root-domain"]; !ok {
		t.Errorf("root-domain was incorrectly deleted")
	}
}

func TestSyncNodeAndRename(t *testing.T) {
	mock, srv := newMockCFServer()
	defer srv.Close()

	client := NewClient("zone-123", "secret-token", "easy42.example.com").WithBaseURL(srv.URL)

	dnsCfg := config.DNSConfig{
		ZoneID:             "zone-123",
		APIToken:           "secret-token",
		BaseDomain:         "easy42.example.com",
		PublishIPv6OwnName: true,
	}

	node := config.Node{Name: "oldnode", IP: "10.0.0.1", IP6: "fd42::1"}
	if err := SyncNode(context.Background(), client, node, "", dnsCfg); err != nil {
		t.Fatalf("SyncNode failed: %v", err)
	}

	// When PublishIPv6OwnName is true, expect 3 records: oldnode A, *.oldnode A, oldnode6 AAAA
	if len(mock.records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(mock.records))
	}

	// Rename node to newnode
	newNode := config.Node{Name: "newnode", IP: "10.0.0.1", IP6: "fd42::1"}
	if err := SyncNode(context.Background(), client, newNode, "oldnode", dnsCfg); err != nil {
		t.Fatalf("SyncNode rename failed: %v", err)
	}

	for _, r := range mock.records {
		if strings.Contains(r.Name, "oldnode") {
			t.Errorf("oldnode record was not deleted: %s", r.Name)
		}
		if !strings.Contains(r.Name, "newnode") {
			t.Errorf("unexpected record name: %s", r.Name)
		}
	}

	// Delete node
	if err := DeleteNodeRecords(context.Background(), client, "newnode", dnsCfg); err != nil {
		t.Fatalf("DeleteNodeRecords failed: %v", err)
	}

	if len(mock.records) != 0 {
		t.Errorf("expected 0 records after delete, got %d", len(mock.records))
	}
}

func TestSyncCustomRecordsAndOverride(t *testing.T) {
	mock, srv := newMockCFServer()
	defer srv.Close()

	client := NewClient("zone-123", "secret-token", "easy42.example.com").WithBaseURL(srv.URL)

	nodes := []config.Node{
		{
			Name:       "node1",
			IP:         "10.0.0.1",
			IP6:        "fd42::1",
			ExternalIP: "203.0.113.10",
			Tags:       []string{"web"},
			Note:       "```json\n{\"custom_ttl\": \"120\", \"svc_port\": 8443}\n```",
		},
		{
			Name: "node2",
			IP:   "10.0.0.2",
			Tags: []string{"web"},
		},
	}

	dnsCfg := config.DNSConfig{
		ZoneID:             "zone-123",
		APIToken:           "secret-token",
		BaseDomain:         "easy42.example.com",
		PublishIPv6OwnName: true,
		CustomRecords: []config.DNSCustomRecordRule{
			{
				// Overrides built-in node1.easy42.example.com A record with ExternalIP and custom TTL
				TargetNodes: map[string]string{"name": "node1"},
				Name:        "${NODE_NAME}.${BASE}",
				Type:        "A",
				Value:       "${NODE_EXTERNAL_IP}",
				TTL:         "${NODE_DATA_CUSTOM_TTL}",
			},
			{
				// Auto-appends .example.com because it doesn't end with .example.com
				TargetNodes: map[string]string{"tag": "web"},
				Name:        "web",
				Type:        "A",
				Value:       "${NODE_IP}",
				TTL:         "Auto",
			},
			{
				// Uses ${ROOT} explicitly and TXT type
				TargetNodes: map[string]string{"name": "node1"},
				Name:        "_info.${ROOT}",
				Type:        "TXT",
				Value:       "node=${NODE_NAME},env=${GLOBAL_ENV}",
				TTL:         "300",
			},
			{
				// SRV record with Cloudflare API JSON data format
				TargetNodes: map[string]string{"name": "node1"},
				Name:        "_https._tcp.${BASE}",
				Type:        "SRV",
				Value:       `{"priority": 10, "weight": 5, "port": ${NODE_DATA_SVC_PORT}, "target": "${NODE_NAME}.${BASE}"}`,
			},
			{
				// CNAME record under base domain (must not be deleted by Force Sync)
				TargetNodes: map[string]string{"name": "node2"},
				Name:        "alias.${BASE}",
				Type:        "CNAME",
				Value:       "${NODE_NAME}.${BASE}",
			},
		},
	}

	globalVars := map[string]string{"GLOBAL_ENV": "prod"}

	// Force sync: creates built-in + custom records without deleting custom records under easy42.example.com
	res, err := Sync(context.Background(), client, nodes, dnsCfg, true, globalVars)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected sync errors: %v", res.Errors)
	}

	findAll := func(rType, name string) []DNSRecord {
		var out []DNSRecord
		for _, r := range mock.records {
			if strings.EqualFold(r.Type, rType) && strings.EqualFold(r.Name, name) {
				out = append(out, r)
			}
		}
		return out
	}

	// 1. Check override of node1.easy42.example.com A record
	n1A := findAll("A", "node1.easy42.example.com")
	if len(n1A) != 1 || n1A[0].Content != "203.0.113.10" || n1A[0].TTL != 120 {
		t.Fatalf("expected overridden A record for node1 (203.0.113.10, ttl=120), got: %+v", n1A)
	}

	// 2. Check auto-appended .example.com and multi-node A records at web.example.com
	webA := findAll("A", "web.example.com")
	if len(webA) != 2 {
		t.Fatalf("expected 2 A records at web.example.com, got: %+v", webA)
	}

	// 3. Check TXT record with ${ROOT} and global var
	txtRec := findAll("TXT", "_info.example.com")
	if len(txtRec) != 1 || txtRec[0].Content != "node=node1,env=prod" || txtRec[0].TTL != 300 {
		t.Fatalf("unexpected TXT record: %+v", txtRec)
	}

	// 4. Check SRV record
	srvRec := findAll("SRV", "_https._tcp.easy42.example.com")
	if len(srvRec) != 1 || len(srvRec[0].Data) == 0 {
		t.Fatalf("unexpected SRV record: %+v", srvRec)
	}
	if !srvDataEqual(srvRec[0], DNSRecord{Type: "SRV", Data: map[string]any{"priority": 10, "weight": 5, "port": 8443, "target": "node1.easy42.example.com"}}) {
		t.Fatalf("SRV record data mismatch: %+v", srvRec[0].Data)
	}

	// 5. Check CNAME record
	cnameRec := findAll("CNAME", "alias.easy42.example.com")
	if len(cnameRec) != 1 || cnameRec[0].Content != "node2.easy42.example.com" || cnameRec[0].TTL != 1 {
		t.Fatalf("unexpected CNAME record: %+v", cnameRec)
	}

	// 6. Second sync should report 0 created, 0 updated, 0 deleted, all ignored
	res2, err := Sync(context.Background(), client, nodes, dnsCfg, true, globalVars)
	if err != nil {
		t.Fatalf("second Sync failed: %v", err)
	}
	if res2.Created != 0 || res2.Updated != 0 || res2.Deleted != 0 {
		t.Fatalf("expected no changes on second sync, got: %+v", res2)
	}
}

func TestSyncSSHFPRecords(t *testing.T) {
	mock, srv := newMockCFServer()
	defer srv.Close()

	client := NewClient("zone-123", "secret-token", "easy42.example.com").WithBaseURL(srv.URL)

	ed25519Blob := base64.StdEncoding.EncodeToString([]byte("fake-ed25519-host-key"))
	ecdsaBlob := base64.StdEncoding.EncodeToString([]byte("fake-ecdsa-host-key"))
	ed448Blob := base64.StdEncoding.EncodeToString([]byte("fake-ed448-host-key"))
	rsaBlob := base64.StdEncoding.EncodeToString([]byte("fake-rsa-host-key"))
	userBlob := base64.StdEncoding.EncodeToString([]byte("fake-ed25519-user-key"))

	sumHex := func(raw string) string {
		b, _ := base64.StdEncoding.DecodeString(raw)
		s := sha256.Sum256(b)
		return hex.EncodeToString(s[:])
	}

	nodes := []config.Node{
		{Name: "node1", IP: "10.0.0.1", Tags: []string{"ssh"}},
		{Name: "node2", IP: "10.0.0.2", Tags: []string{"other"}},
	}

	hostKeysByNode := map[string][]config.SSHPublicKey{
		"node1": {
			{Algorithm: "ssh-ed25519", Key: ed25519Blob, Kind: config.SSHKeyKindHost},
			{Algorithm: "ecdsa-sha2-nistp256", Key: ecdsaBlob, Kind: config.SSHKeyKindHost},
			{Algorithm: "ssh-ed448", Key: ed448Blob, Kind: config.SSHKeyKindHost},
			{Algorithm: "ssh-rsa", Key: rsaBlob, Kind: config.SSHKeyKindHost},    // legacy, ignored
			{Algorithm: "ssh-ed25519", Key: userBlob, Kind: config.SSHKeyKindUser}, // user key, ignored
		},
		"node2": {
			{Algorithm: "ssh-ed25519", Key: ed25519Blob, Kind: config.SSHKeyKindHost},
		},
	}

	dnsCfg := config.DNSConfig{
		ZoneID:             "zone-123",
		APIToken:           "secret-token",
		BaseDomain:         "easy42.example.com",
		PublishIPv6OwnName: true,
		SSHFPRules: []config.DNSSSHFPRule{
			{
				TargetNodes: map[string]string{"tag": "ssh"},
				Algorithms:  []int{3, 4}, // ECDSA and Ed25519 only (not 6 Ed448)
			},
		},
	}

	res, err := SyncWithSSHKeys(context.Background(), client, nodes, dnsCfg, true, nil, hostKeysByNode)
	if err != nil {
		t.Fatalf("SyncWithSSHKeys failed: %v", err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected sync errors: %v", res.Errors)
	}

	var sshfpRecs []DNSRecord
	for _, r := range mock.records {
		if strings.EqualFold(r.Type, "SSHFP") {
			sshfpRecs = append(sshfpRecs, r)
		}
	}

	// Only node1 matches tag "ssh", and only algorithms 3 (ECDSA) and 4 (Ed25519) are selected
	if len(sshfpRecs) != 2 {
		t.Fatalf("expected 2 SSHFP records for node1, got %d: %+v", len(sshfpRecs), sshfpRecs)
	}

	wantEd25519 := sumHex(ed25519Blob)
	wantECDSA := sumHex(ecdsaBlob)
	foundEd25519, foundECDSA := false, false
	for _, r := range sshfpRecs {
		if r.Name != "node1.easy42.example.com" {
			t.Fatalf("unexpected SSHFP record name: %s", r.Name)
		}
		alg, ft, fp, ok := extractSSHFPTuple(r)
		if !ok || ft != 2 {
			t.Fatalf("invalid SSHFP data (expected hash type 2): %+v", r.Data)
		}
		if alg == 4 && fp == wantEd25519 {
			foundEd25519 = true
		}
		if alg == 3 && fp == wantECDSA {
			foundECDSA = true
		}
	}
	if !foundEd25519 || !foundECDSA {
		t.Fatalf("missing expected SSHFP records: ed25519=%v ecdsa=%v", foundEd25519, foundECDSA)
	}

	// Second sync should be idempotent
	res2, err := SyncWithSSHKeys(context.Background(), client, nodes, dnsCfg, true, nil, hostKeysByNode)
	if err != nil {
		t.Fatalf("second SyncWithSSHKeys failed: %v", err)
	}
	if res2.Created != 0 || res2.Updated != 0 || res2.Deleted != 0 {
		t.Fatalf("expected no changes on second sync, got: %+v", res2)
	}

	// Validation rejects legacy algorithm 1 (RSA)
	badCfg := dnsCfg
	badCfg.SSHFPRules = []config.DNSSSHFPRule{{TargetNodes: config.AllNodesSelector(), Algorithms: []int{1}}}
	if err := config.ValidateDNSConfig(&badCfg); err == nil {
		t.Fatalf("expected ValidateDNSConfig to reject legacy algorithm 1 (RSA)")
	}
}
