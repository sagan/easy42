package dns

import (
	"context"
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
	aRec, aaaaRec := ExpectedRecordsForNode(node, "easy42.example.com", false)
	if aRec == nil || aRec.Name != "node1.easy42.example.com" || aRec.Content != "192.168.100.1" {
		t.Fatalf("unexpected aRec: %+v", aRec)
	}
	if aaaaRec == nil || aaaaRec.Name != "node1.easy42.example.com" || aaaaRec.Content != "fd42:a159:f9f0::1" {
		t.Fatalf("unexpected aaaaRec when toggle false: %+v", aaaaRec)
	}

	// Toggle ON
	aRec2, aaaaRec2 := ExpectedRecordsForNode(node, "easy42.example.com", true)
	if aRec2 == nil || aRec2.Name != "node1.easy42.example.com" {
		t.Fatalf("unexpected aRec2: %+v", aRec2)
	}
	if aaaaRec2 == nil || aaaaRec2.Name != "node16.easy42.example.com" || aaaaRec2.Content != "fd42:a159:f9f0::1" {
		t.Fatalf("unexpected aaaaRec2 when toggle true: %+v", aaaaRec2)
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

	// 1. Initial sync (creates all 4 records)
	res, err := Sync(context.Background(), client, nodes, dnsCfg, false)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if res.Created != 4 {
		t.Errorf("expected 4 created, got %d", res.Created)
	}
	if len(mock.records) != 4 {
		t.Errorf("expected 4 records in mock, got %d", len(mock.records))
	}

	// 2. Second sync with no changes (should ignore all 4)
	res2, err := Sync(context.Background(), client, nodes, dnsCfg, false)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if res2.Ignored != 4 {
		t.Errorf("expected 4 ignored, got %d", res2.Ignored)
	}

	// 3. Update an IP
	nodes[0].IP = "10.0.0.10"
	res3, err := Sync(context.Background(), client, nodes, dnsCfg, false)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if res3.Updated != 1 || res3.Ignored != 3 {
		t.Errorf("expected 1 updated, 3 ignored, got updated=%d, ignored=%d", res3.Updated, res3.Ignored)
	}

	// 4. Toggle PublishIPv6OwnName = true
	dnsCfg.PublishIPv6OwnName = true
	res4, err := Sync(context.Background(), client, nodes, dnsCfg, false)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	// Should create 2 new AAAA at n16 and n26, ignore 2 A records, and delete old 2 AAAA at n1 and n2
	if res4.Created != 2 || res4.Deleted != 2 || res4.Ignored != 2 {
		t.Errorf("expected 2 created, 2 deleted, 2 ignored, got created=%d, deleted=%d, ignored=%d",
			res4.Created, res4.Deleted, res4.Ignored)
	}
}

func TestForceSyncDeletesOrphanedRecords(t *testing.T) {
	mock, srv := newMockCFServer()
	defer srv.Close()

	client := NewClient("zone-123", "secret-token", "easy42.example.com").WithBaseURL(srv.URL)

	// Add an orphaned record to mock that doesn't correspond to any node
	mock.records["orphan-1"] = DNSRecord{
		ID:      "orphan-1",
		Type:    "A",
		Name:    "ghost.easy42.example.com",
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

	if res.Deleted != 2 {
		t.Errorf("expected 2 orphans deleted, got %d", res.Deleted)
	}

	if _, ok := mock.records["orphan-1"]; ok {
		t.Errorf("orphan-1 was not deleted")
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

	if len(mock.records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(mock.records))
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
