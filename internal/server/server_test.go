package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"easy42/internal/config"
	"easy42/internal/engine"
)

func setupTestServer(t *testing.T) (*Server, string, string) {
	tempDir, err := os.MkdirTemp("", "easy42-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := engine.NewManager(store)
	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Manager:    mgr,
	})

	return srv, tempDir, pass
}

func TestChangePasswordAndLogoutAll(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// 1. Test Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Login failed: code %d, body: %s", w.Code, w.Body.String())
	}

	cookie := w.Result().Cookies()[0]
	if cookie.Name != "easy42_session" || cookie.Value == "" {
		t.Fatalf("Expected valid cookie, got %v", cookie)
	}

	// 2. Test ChangePassword with incorrect current password
	badChangeBody, _ := json.Marshal(map[string]string{
		"current_password": "wrongpassword123",
		"new_password":     "newpassword12345",
	})
	reqBad := httptest.NewRequest("POST", "/api/auth/change-password", bytes.NewReader(badChangeBody))
	reqBad.AddCookie(cookie)
	wBad := httptest.NewRecorder()
	srv.router.ServeHTTP(wBad, reqBad)

	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("Expected bad request for wrong old password, got code %d", wBad.Code)
	}

	// 3. Test ChangePassword with correct current password
	newPass := "validNewPassword999!"
	goodChangeBody, _ := json.Marshal(map[string]string{
		"current_password": initPass,
		"new_password":     newPass,
	})
	reqGood := httptest.NewRequest("POST", "/api/auth/change-password", bytes.NewReader(goodChangeBody))
	reqGood.AddCookie(cookie)
	wGood := httptest.NewRecorder()
	srv.router.ServeHTTP(wGood, reqGood)

	if wGood.Code != http.StatusOK {
		t.Fatalf("Expected 200 on change password, got %d: %s", wGood.Code, wGood.Body.String())
	}

	// New cookie was set
	cookies := wGood.Result().Cookies()
	var newCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "easy42_session" {
			newCookie = c
			break
		}
	}
	if newCookie == nil || newCookie.Value == "" {
		t.Fatalf("Expected updated session cookie after password change")
	}

	// Old cookie should now be invalid because password_hash changed
	reqOldCookie := httptest.NewRequest("GET", "/api/nodes", nil)
	reqOldCookie.AddCookie(cookie)
	wOldCookie := httptest.NewRecorder()
	srv.router.ServeHTTP(wOldCookie, reqOldCookie)
	if wOldCookie.Code != http.StatusUnauthorized {
		t.Fatalf("Expected old cookie to be unauthorized, got %d", wOldCookie.Code)
	}

	// New cookie should be valid
	reqNewCookie := httptest.NewRequest("GET", "/api/nodes", nil)
	reqNewCookie.AddCookie(newCookie)
	wNewCookie := httptest.NewRecorder()
	srv.router.ServeHTTP(wNewCookie, reqNewCookie)
	if wNewCookie.Code != http.StatusOK {
		t.Fatalf("Expected new cookie to be authorized, got %d", wNewCookie.Code)
	}

	// 4. Test Logout All
	reqLogoutAll := httptest.NewRequest("POST", "/api/auth/logout-all", nil)
	reqLogoutAll.AddCookie(newCookie)
	wLogoutAll := httptest.NewRecorder()
	srv.router.ServeHTTP(wLogoutAll, reqLogoutAll)

	if wLogoutAll.Code != http.StatusOK {
		t.Fatalf("Expected 200 on logout all, got %d: %s", wLogoutAll.Code, wLogoutAll.Body.String())
	}

	// Verify session secret was changed in config.json
	cfgData, err := os.ReadFile(filepath.Join(tempDir, "config.json"))
	if err != nil {
		t.Fatalf("Failed to read config.json: %v", err)
	}
	var cfg config.Config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}
	if cfg.SessionSecret == "" {
		t.Fatalf("Session secret is empty")
	}

	// New cookie should now ALSO be unauthorized because session_secret was reset
	reqAfterLogoutAll := httptest.NewRequest("GET", "/api/nodes", nil)
	reqAfterLogoutAll.AddCookie(newCookie)
	wAfterLogoutAll := httptest.NewRecorder()
	srv.router.ServeHTTP(wAfterLogoutAll, reqAfterLogoutAll)
	if wAfterLogoutAll.Code != http.StatusUnauthorized {
		t.Fatalf("Expected cookie to be unauthorized after logout all, got %d", wAfterLogoutAll.Code)
	}
}

func TestUpdateLinkAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	cookie := w.Result().Cookies()[0]

	// Add two nodes
	nodeA := config.Node{Name: "n1", Host: "1.1.1.1", IP: "10.0.0.1", Interface: "lo", ASN: 4224420001}
	nodeB := config.Node{Name: "n2", Host: "2.2.2.2", IP: "10.0.0.2", Interface: "lo", ASN: 4224420002}
	bodyA, _ := json.Marshal(nodeA)
	reqA := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(bodyA))
	reqA.AddCookie(cookie)
	wA := httptest.NewRecorder()
	srv.router.ServeHTTP(wA, reqA)

	bodyB, _ := json.Marshal(nodeB)
	reqB := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(bodyB))
	reqB.AddCookie(cookie)
	wB := httptest.NewRecorder()
	srv.router.ServeHTTP(wB, reqB)

	// Add link
	linkReq := map[string]any{
		"from_node": "n1",
		"to_node":   "n2",
		"from_port": 50001,
		"to_port":   50002,
		"from_mtu":  1400,
		"to_mtu":    1400,
		"from_cost":       35,
		"to_cost":         45,
		"from_fwmark":     "51820",
		"to_fwmark":       "0xca64",
		"from_preference": 120,
		"to_preference":   180,
		"from_mark":       "0x1234",
		"to_mark":         "42",
	}
	bodyLink, _ := json.Marshal(linkReq)
	reqLink := httptest.NewRequest("POST", "/api/links", bytes.NewReader(bodyLink))
	reqLink.AddCookie(cookie)
	wLink := httptest.NewRecorder()
	srv.router.ServeHTTP(wLink, reqLink)
	if wLink.Code != http.StatusCreated {
		t.Fatalf("AddLink failed: %d %s", wLink.Code, wLink.Body.String())
	}

	var added config.Link
	if err := json.Unmarshal(wLink.Body.Bytes(), &added); err != nil {
		t.Fatalf("Failed to decode added link response: %v", err)
	}
	if added.From.Cost != 35 || added.To.Cost != 45 {
		t.Errorf("Unexpected added link costs: from=%d, to=%d", added.From.Cost, added.To.Cost)
	}
	if added.From.Fwmark != "51820" || added.To.Fwmark != "0xca64" {
		t.Errorf("Unexpected added link fwmark: from=%q, to=%q", added.From.Fwmark, added.To.Fwmark)
	}
	if added.From.Preference == nil || *added.From.Preference != 120 || added.To.Preference == nil || *added.To.Preference != 180 {
		t.Errorf("Unexpected added link preference: from=%v, to=%v", added.From.Preference, added.To.Preference)
	}
	if added.From.Mark != "0x1234" || added.To.Mark != "42" {
		t.Errorf("Unexpected added link mark: from=%q, to=%q", added.From.Mark, added.To.Mark)
	}

	// Update link
	updateReq := map[string]any{
		"from_node": "n1",
		"to_node":   "n2",
		"from_port": 51000,
		"to_port":   52000,
		"from_mtu":  1360,
		"to_mtu":    1360,
		"from_cost":       80,
		"from_fwmark":     "0x1234",
		"from_preference": 220,
		"from_mark":       "0x9999",
	}
	bodyUpdate, _ := json.Marshal(updateReq)
	reqUpdate := httptest.NewRequest("PUT", "/api/links", bytes.NewReader(bodyUpdate))
	reqUpdate.AddCookie(cookie)
	wUpdate := httptest.NewRecorder()
	srv.router.ServeHTTP(wUpdate, reqUpdate)

	if wUpdate.Code != http.StatusOK {
		t.Fatalf("UpdateLink failed: %d %s", wUpdate.Code, wUpdate.Body.String())
	}

	var updated config.Link
	if err := json.Unmarshal(wUpdate.Body.Bytes(), &updated); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if updated.From.ListenPort != 51000 || updated.To.ListenPort != 52000 {
		t.Errorf("Unexpected ports: %d, %d", updated.From.ListenPort, updated.To.ListenPort)
	}
	if updated.From.MTU != 1360 || updated.To.MTU != 1360 {
		t.Errorf("Unexpected MTUs: %d, %d", updated.From.MTU, updated.To.MTU)
	}
	if updated.From.Cost != 80 {
		t.Errorf("Unexpected updated From.Cost: %d (expected 80)", updated.From.Cost)
	}
	if updated.From.Fwmark != "0x1234" {
		t.Errorf("Unexpected updated From.Fwmark: %q (expected 0x1234)", updated.From.Fwmark)
	}
	if updated.From.Preference == nil || *updated.From.Preference != 220 {
		t.Errorf("Unexpected updated From.Preference: %v (expected 220)", updated.From.Preference)
	}
	if updated.From.Mark != "0x9999" {
		t.Errorf("Unexpected updated From.Mark: %q (expected 0x9999)", updated.From.Mark)
	}
}

func TestUpdateNodePositionAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	reqLogin := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	wLogin := httptest.NewRecorder()
	srv.router.ServeHTTP(wLogin, reqLogin)
	cookie := wLogin.Result().Cookies()[0]

	// Add node
	node := config.Node{
		Name:      "test-pos",
		Host:      "192.168.1.50",
		IP:        "192.168.100.50",
		Interface: "lo",
		ASN:       4224420050,
	}
	bodyNode, _ := json.Marshal(node)
	reqNode := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(bodyNode))
	reqNode.AddCookie(cookie)
	wNode := httptest.NewRecorder()
	srv.router.ServeHTTP(wNode, reqNode)
	if wNode.Code != http.StatusCreated {
		t.Fatalf("AddNode failed: %d %s", wNode.Code, wNode.Body.String())
	}

	// Update position via PUT /api/nodes/test-pos/position
	posBody, _ := json.Marshal(map[string]float64{"x": 620.0, "y": 380.5})
	reqPos := httptest.NewRequest("PUT", "/api/nodes/test-pos/position", bytes.NewReader(posBody))
	reqPos.AddCookie(cookie)
	wPos := httptest.NewRecorder()
	srv.router.ServeHTTP(wPos, reqPos)
	if wPos.Code != http.StatusOK {
		t.Fatalf("UpdateNodePosition PUT failed: %d %s", wPos.Code, wPos.Body.String())
	}

	// Get nodes and check coordinates
	reqGet := httptest.NewRequest("GET", "/api/nodes", nil)
	reqGet.AddCookie(cookie)
	wGet := httptest.NewRecorder()
	srv.router.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("GetNodes failed: %d %s", wGet.Code, wGet.Body.String())
	}

	var nodes []config.Node
	if err := json.Unmarshal(wGet.Body.Bytes(), &nodes); err != nil {
		t.Fatalf("Failed to decode nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("Expected 1 node, got %d", len(nodes))
	}
	if nodes[0].X == nil || *nodes[0].X != 620.0 || nodes[0].Y == nil || *nodes[0].Y != 380.5 {
		t.Fatalf("Expected X=620.0, Y=380.5, got X=%v, Y=%v", nodes[0].X, nodes[0].Y)
	}

	// Also test PATCH /api/nodes/test-pos/position
	patchBody, _ := json.Marshal(map[string]float64{"x": 700.0, "y": 400.0})
	reqPatch := httptest.NewRequest("PATCH", "/api/nodes/test-pos/position", bytes.NewReader(patchBody))
	reqPatch.AddCookie(cookie)
	wPatch := httptest.NewRecorder()
	srv.router.ServeHTTP(wPatch, reqPatch)
	if wPatch.Code != http.StatusOK {
		t.Fatalf("UpdateNodePosition PATCH failed: %d %s", wPatch.Code, wPatch.Body.String())
	}

	reqGet2 := httptest.NewRequest("GET", "/api/nodes", nil)
	reqGet2.AddCookie(cookie)
	wGet2 := httptest.NewRecorder()
	srv.router.ServeHTTP(wGet2, reqGet2)
	nodes = nil
	_ = json.Unmarshal(wGet2.Body.Bytes(), &nodes)
	if nodes[0].X == nil || *nodes[0].X != 700.0 || nodes[0].Y == nil || *nodes[0].Y != 400.0 {
		t.Fatalf("Expected X=700.0, Y=400.0, got X=%v, Y=%v", nodes[0].X, nodes[0].Y)
	}
}

func TestSyncMeshEmptyGraph(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Login failed: %d %s", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]

	// 1. Test GET /api/sync/preview with empty graph
	reqPreview := httptest.NewRequest("GET", "/api/sync/preview", nil)
	reqPreview.AddCookie(cookie)
	wPreview := httptest.NewRecorder()
	srv.router.ServeHTTP(wPreview, reqPreview)

	if wPreview.Code != http.StatusOK {
		t.Fatalf("Sync preview failed: %d %s", wPreview.Code, wPreview.Body.String())
	}
	if strings.TrimSpace(wPreview.Body.String()) != "[]" {
		t.Fatalf("Expected [] from sync preview on empty graph, got: %s", wPreview.Body.String())
	}

	// 2. Test POST /api/sync with empty graph
	reqSync := httptest.NewRequest("POST", "/api/sync", nil)
	reqSync.AddCookie(cookie)
	wSync := httptest.NewRecorder()
	srv.router.ServeHTTP(wSync, reqSync)

	if wSync.Code != http.StatusOK {
		t.Fatalf("Sync execute failed: %d %s", wSync.Code, wSync.Body.String())
	}
	if strings.TrimSpace(wSync.Body.String()) != "[]" {
		t.Fatalf("Expected [] from sync execute on empty graph, got: %s", wSync.Body.String())
	}

	// 3. Test POST /api/sync?force=true with empty graph
	reqSyncForce := httptest.NewRequest("POST", "/api/sync?force=true", nil)
	reqSyncForce.AddCookie(cookie)
	wSyncForce := httptest.NewRecorder()
	srv.router.ServeHTTP(wSyncForce, reqSyncForce)

	if wSyncForce.Code != http.StatusOK {
		t.Fatalf("Sync force failed: %d %s", wSyncForce.Code, wSyncForce.Body.String())
	}

	// 3b. Test GET /api/sync/preview?node=node-1
	reqPreviewNode := httptest.NewRequest("GET", "/api/sync/preview?node=node-1", nil)
	reqPreviewNode.AddCookie(cookie)
	wPreviewNode := httptest.NewRecorder()
	srv.router.ServeHTTP(wPreviewNode, reqPreviewNode)
	if wPreviewNode.Code != http.StatusOK {
		t.Fatalf("Sync preview with node failed: %d %s", wPreviewNode.Code, wPreviewNode.Body.String())
	}

	// 3c. Test POST /api/sync?node=node-1
	reqSyncNode := httptest.NewRequest("POST", "/api/sync?node=node-1", nil)
	reqSyncNode.AddCookie(cookie)
	wSyncNode := httptest.NewRecorder()
	srv.router.ServeHTTP(wSyncNode, reqSyncNode)
	if wSyncNode.Code != http.StatusOK {
		t.Fatalf("Sync execute with node failed: %d %s", wSyncNode.Code, wSyncNode.Body.String())
	}

	// 4. Test GET /api/state
	reqState := httptest.NewRequest("GET", "/api/state", nil)
	reqState.AddCookie(cookie)
	wState := httptest.NewRecorder()
	srv.router.ServeHTTP(wState, reqState)

	if wState.Code != http.StatusOK {
		t.Fatalf("Get state failed: %d %s", wState.Code, wState.Body.String())
	}

	// 5. Test POST /api/state/update
	reqStateUpdate := httptest.NewRequest("POST", "/api/state/update", nil)
	reqStateUpdate.AddCookie(cookie)
	wStateUpdate := httptest.NewRecorder()
	srv.router.ServeHTTP(wStateUpdate, reqStateUpdate)

	if wStateUpdate.Code != http.StatusOK {
		t.Fatalf("Update state failed: %d %s", wStateUpdate.Code, wStateUpdate.Body.String())
	}
}

func TestCreateMeshLinksAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	reqLogin := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	wLogin := httptest.NewRecorder()
	srv.router.ServeHTTP(wLogin, reqLogin)
	cookie := wLogin.Result().Cookies()[0]

	// Add 3 nodes
	nodes := []config.Node{
		{Name: "n1", Host: "10.0.0.1", IP: "192.168.100.1", Interface: "lo", ASN: 4224420001, Tags: []string{"zone1"}},
		{Name: "n2", Host: "10.0.0.2", IP: "192.168.100.2", Interface: "lo", ASN: 4224420002, Tags: []string{"zone1"}},
		{Name: "n3", Host: "10.0.0.3", IP: "192.168.100.3", Interface: "lo", ASN: 4224420003, Tags: []string{"zone2"}},
	}
	for _, n := range nodes {
		b, _ := json.Marshal(n)
		req := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(b))
		req.AddCookie(cookie)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("AddNode failed: %d %s", w.Code, w.Body.String())
		}
	}

	// Create mesh for subset ["n1", "n2"]
	meshBody, _ := json.Marshal(map[string]any{"nodes": []string{"n1", "n2"}})
	reqMesh := httptest.NewRequest("POST", "/api/links/mesh", bytes.NewReader(meshBody))
	reqMesh.AddCookie(cookie)
	wMesh := httptest.NewRecorder()
	srv.router.ServeHTTP(wMesh, reqMesh)

	if wMesh.Code != http.StatusCreated {
		t.Fatalf("CreateMesh failed: %d %s", wMesh.Code, wMesh.Body.String())
	}

	var createdLinks []*config.Link
	if err := json.Unmarshal(wMesh.Body.Bytes(), &createdLinks); err != nil {
		t.Fatalf("Unmarshal created links failed: %v", err)
	}
	if len(createdLinks) != 1 {
		t.Fatalf("Expected 1 link created between n1 and n2, got %d", len(createdLinks))
	}

	// Now create mesh for all nodes
	reqMeshAll := httptest.NewRequest("POST", "/api/links/mesh", bytes.NewReader([]byte("{}")))
	reqMeshAll.AddCookie(cookie)
	wMeshAll := httptest.NewRecorder()
	srv.router.ServeHTTP(wMeshAll, reqMeshAll)

	if wMeshAll.Code != http.StatusCreated {
		t.Fatalf("CreateMeshAll failed: %d %s", wMeshAll.Code, wMeshAll.Body.String())
	}

	var createdLinksAll []*config.Link
	if err := json.Unmarshal(wMeshAll.Body.Bytes(), &createdLinksAll); err != nil {
		t.Fatalf("Unmarshal created links all failed: %v", err)
	}
	if len(createdLinksAll) != 2 {
		t.Fatalf("Expected 2 additional links created for full mesh of 3, got %d", len(createdLinksAll))
	}
}

func TestGetNodeBirdConfigAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	cookie := w.Result().Cookies()[0]

	// Add node
	node := config.Node{
		Name:      "gateway1",
		Host:      "192.168.1.1",
		IP:        "192.168.50.100",
		Interface: "lo",
		ASN:       4224420001,
		StaticRoutes: []string{
			"192.168.50.0/24",
		},
		Routes: []config.KernelRouteRule{
			{
				Table:    100,
				Prefixes: []string{"10.0.0.0/8+"},
			},
		},
	}
	bodyNode, _ := json.Marshal(node)
	reqNode := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(bodyNode))
	reqNode.AddCookie(cookie)
	wNode := httptest.NewRecorder()
	srv.router.ServeHTTP(wNode, reqNode)
	if wNode.Code != http.StatusCreated {
		t.Fatalf("AddNode failed: %d %s", wNode.Code, wNode.Body.String())
	}

	// 1. Test JSON response
	reqBird := httptest.NewRequest("GET", "/api/nodes/gateway1/bird", nil)
	reqBird.AddCookie(cookie)
	wBird := httptest.NewRecorder()
	srv.router.ServeHTTP(wBird, reqBird)

	if wBird.Code != http.StatusOK {
		t.Fatalf("GetBirdConfig failed: %d %s", wBird.Code, wBird.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(wBird.Body.Bytes(), &res); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
	confStr, ok := res["config"].(string)
	if !ok || !strings.Contains(confStr, "define SELF_IP = 192.168.50.100;") {
		t.Fatalf("Unexpected bird config content: %v", res)
	}

	// 2. Test raw text response
	reqBirdRaw := httptest.NewRequest("GET", "/api/nodes/gateway1/bird?raw=true", nil)
	reqBirdRaw.AddCookie(cookie)
	wBirdRaw := httptest.NewRecorder()
	srv.router.ServeHTTP(wBirdRaw, reqBirdRaw)

	if wBirdRaw.Code != http.StatusOK {
		t.Fatalf("GetBirdConfig raw failed: %d %s", wBirdRaw.Code, wBirdRaw.Body.String())
	}
	if !strings.Contains(wBirdRaw.Body.String(), "define SELF_AS = 4224420001;") {
		t.Fatalf("Expected raw bird config with SELF_AS, got:\n%s", wBirdRaw.Body.String())
	}
}

func TestTasksAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	cookie := w.Result().Cookies()[0]

	// 1. GET /api/tasks
	reqTasks := httptest.NewRequest("GET", "/api/tasks", nil)
	reqTasks.AddCookie(cookie)
	wTasks := httptest.NewRecorder()
	srv.router.ServeHTTP(wTasks, reqTasks)

	if wTasks.Code != http.StatusOK {
		t.Fatalf("GetTasks failed: %d %s", wTasks.Code, wTasks.Body.String())
	}

	var taskList []map[string]any
	if err := json.Unmarshal(wTasks.Body.Bytes(), &taskList); err != nil {
		t.Fatalf("Failed to parse tasks response: %v", err)
	}

	if len(taskList) < 5 {
		t.Fatalf("Expected at least 5 tasks, got %d", len(taskList))
	}

	// 2. Test unknown task status
	reqBadTask := httptest.NewRequest("POST", "/api/tasks/unknown_task_123/status", nil)
	reqBadTask.AddCookie(cookie)
	wBadTask := httptest.NewRecorder()
	srv.router.ServeHTTP(wBadTask, reqBadTask)

	if wBadTask.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for unknown task, got %d", wBadTask.Code)
	}
}

func TestNetworkSettingsAndExternalPeeringAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	cookie := w.Result().Cookies()[0]

	// 1. GET /api/settings/network (initially empty)
	reqGetSettings := httptest.NewRequest("GET", "/api/settings/network", nil)
	reqGetSettings.AddCookie(cookie)
	wGetSettings := httptest.NewRecorder()
	srv.router.ServeHTTP(wGetSettings, reqGetSettings)
	if wGetSettings.Code != http.StatusOK {
		t.Fatalf("GetNetworkSettings failed: %d %s", wGetSettings.Code, wGetSettings.Body.String())
	}

	// 2. PUT /api/settings/network
	settingsUpdate := config.NetworkSettings{
		PublicASN:              4242421234,
		Prefixes:               []string{"172.20.10.0/24", "172.20.0.0/14{21,29}"},
		LocalDN42Networks:      []string{"172.20.229.0/27"},
		DisallowedDN42Networks: []string{"fd42:1234:5678::/48", "172.20.99.0/24"},
	}
	bodyPutSettings, _ := json.Marshal(settingsUpdate)
	reqPutSettings := httptest.NewRequest("PUT", "/api/settings/network", bytes.NewReader(bodyPutSettings))
	reqPutSettings.AddCookie(cookie)
	wPutSettings := httptest.NewRecorder()
	srv.router.ServeHTTP(wPutSettings, reqPutSettings)
	if wPutSettings.Code != http.StatusOK {
		t.Fatalf("PutNetworkSettings failed: %d %s", wPutSettings.Code, wPutSettings.Body.String())
	}

	var savedSettings config.NetworkSettings
	_ = json.Unmarshal(wPutSettings.Body.Bytes(), &savedSettings)
	if savedSettings.PublicASN != 4242421234 {
		t.Errorf("Expected PublicASN 4242421234, got %d", savedSettings.PublicASN)
	}
	if len(savedSettings.LocalDN42Networks) != 1 || savedSettings.LocalDN42Networks[0] != "172.20.229.0/27" {
		t.Errorf("Expected LocalDN42Networks [172.20.229.0/27], got %v", savedSettings.LocalDN42Networks)
	}
	if len(savedSettings.DisallowedDN42Networks) != 2 || savedSettings.DisallowedDN42Networks[0] != "fd42:1234:5678::/48" {
		t.Errorf("Expected DisallowedDN42Networks [fd42:1234:5678::/48, 172.20.99.0/24], got %v", savedSettings.DisallowedDN42Networks)
	}

	// Verify that built-in dn42 policy automatically inherits DisallowedDstCIDRs & DisallowedSrcCIDRs
	policies := srv.mgr.GetNetworkPolicies()
	var dn42Policy *config.NetworkPolicy
	for i := range policies {
		if policies[i].ID == config.PolicyDN42 {
			dn42Policy = &policies[i]
			break
		}
	}
	if dn42Policy == nil {
		t.Fatalf("built-in dn42 policy not found")
	}
	if len(dn42Policy.DisallowedDstCIDRs) != 2 || dn42Policy.DisallowedDstCIDRs[0] != "fd42:1234:5678::/48" {
		t.Errorf("Expected dn42 policy DisallowedDstCIDRs to inherit global setting, got %v", dn42Policy.DisallowedDstCIDRs)
	}
	if len(dn42Policy.DisallowedSrcCIDRs) != 2 || dn42Policy.DisallowedSrcCIDRs[0] != "fd42:1234:5678::/48" {
		t.Errorf("Expected dn42 policy DisallowedSrcCIDRs to inherit global setting, got %v", dn42Policy.DisallowedSrcCIDRs)
	}

	// 3. Add Managed Node via POST /api/nodes
	nodeManaged := config.Node{
		Name:      "r1",
		Host:      "10.0.0.1",
		IP:        "192.168.100.1",
		Interface: "lo",
		ASN:       4224420001,
	}
	bodyMNode, _ := json.Marshal(nodeManaged)
	reqMNode := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(bodyMNode))
	reqMNode.AddCookie(cookie)
	wMNode := httptest.NewRecorder()
	srv.router.ServeHTTP(wMNode, reqMNode)
	if wMNode.Code != http.StatusCreated {
		t.Fatalf("Add managed node failed: %d %s", wMNode.Code, wMNode.Body.String())
	}

	// 4. Add External Node via POST /api/nodes (no host, no ip)
	nodeExt := config.Node{
		Name:        "extpeer",
		IsExternal:  true,
		ASN:         4242429999,
		Description: "External Peer Router",
	}
	bodyExtNode, _ := json.Marshal(nodeExt)
	reqExtNode := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(bodyExtNode))
	reqExtNode.AddCookie(cookie)
	wExtNode := httptest.NewRecorder()
	srv.router.ServeHTTP(wExtNode, reqExtNode)
	if wExtNode.Code != http.StatusCreated {
		t.Fatalf("Add external node failed: %d %s", wExtNode.Code, wExtNode.Body.String())
	}

	// 5. Add Link between r1 and extpeer with custom ends
	addLinkReq := map[string]any{
		"from_node": "r1",
		"to_node":   "extpeer",
		"from": map[string]any{
			"name":        "r1",
			"listen_port": 51820,
			"address":     "fe80::1001/64",
		},
		"to": map[string]any{
			"name":       "extpeer",
			"endpoint":   "peer.dn42.net:51820",
			"address":    "fe80::9999/64",
			"public_key": "dGhpcy1pcy1hLXRlc3QtcHVibGljLWtleS0xMjM0NQ==",
		},
	}
	bodyLink, _ := json.Marshal(addLinkReq)
	reqLink := httptest.NewRequest("POST", "/api/links", bytes.NewReader(bodyLink))
	reqLink.AddCookie(cookie)
	wLink := httptest.NewRecorder()
	srv.router.ServeHTTP(wLink, reqLink)
	if wLink.Code != http.StatusCreated {
		t.Fatalf("AddLinkAdvanced failed: %d %s", wLink.Code, wLink.Body.String())
	}

	// 6. Test GET BIRD config via /api/nodes/r1/bird
	reqBird := httptest.NewRequest("GET", "/api/nodes/r1/bird", nil)
	reqBird.AddCookie(cookie)
	wBird := httptest.NewRecorder()
	srv.router.ServeHTTP(wBird, reqBird)
	if wBird.Code != http.StatusOK {
		t.Fatalf("GetNodeBirdConfig failed: %d %s", wBird.Code, wBird.Body.String())
	}
	if !strings.Contains(wBird.Body.String(), "confederation CONFED_AS;") {
		t.Errorf("Expected confederation CONFED_AS in bird config: %s", wBird.Body.String())
	}
}

func TestRenameNodeAPI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-srv-rename-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize store: %v", err)
	}

	mgr := engine.NewManager(store)
	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Manager:    mgr,
	})

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": pass})
	reqLogin := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	wLogin := httptest.NewRecorder()
	srv.router.ServeHTTP(wLogin, reqLogin)
	cookie := wLogin.Result().Cookies()[0]

	// Add node1 and node2
	node1 := config.Node{
		Name:      "test-orig",
		Host:      "192.168.1.100",
		IP:        "192.168.100.100",
		Interface: "lo",
		ASN:       4224420100,
	}
	b1, _ := json.Marshal(node1)
	req1 := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(b1))
	req1.AddCookie(cookie)
	w1 := httptest.NewRecorder()
	srv.router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("AddNode 1 failed: %d %s", w1.Code, w1.Body.String())
	}

	node2 := config.Node{
		Name:      "other-node",
		Host:      "192.168.1.101",
		IP:        "192.168.100.101",
		Interface: "lo",
		ASN:       4224420101,
	}
	b2, _ := json.Marshal(node2)
	req2 := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(b2))
	req2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	srv.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("AddNode 2 failed: %d %s", w2.Code, w2.Body.String())
	}

	// 1. Rename nonexistent node -> 404
	renameBody, _ := json.Marshal(map[string]string{"new_name": "any-name"})
	reqNonExistent := httptest.NewRequest("POST", "/api/nodes/doesnotexist/rename", bytes.NewReader(renameBody))
	reqNonExistent.AddCookie(cookie)
	wNonExistent := httptest.NewRecorder()
	srv.router.ServeHTTP(wNonExistent, reqNonExistent)
	if wNonExistent.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for nonexistent node, got %d", wNonExistent.Code)
	}

	// 2. Rename to already existing name -> 400
	dupBody, _ := json.Marshal(map[string]string{"new_name": "other-node"})
	reqDup := httptest.NewRequest("POST", "/api/nodes/test-orig/rename", bytes.NewReader(dupBody))
	reqDup.AddCookie(cookie)
	wDup := httptest.NewRecorder()
	srv.router.ServeHTTP(wDup, reqDup)
	if wDup.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for duplicate name, got %d: %s", wDup.Code, wDup.Body.String())
	}

	// 3. Rename with empty name -> 400
	emptyBody, _ := json.Marshal(map[string]string{"new_name": ""})
	reqEmpty := httptest.NewRequest("POST", "/api/nodes/test-orig/rename", bytes.NewReader(emptyBody))
	reqEmpty.AddCookie(cookie)
	wEmpty := httptest.NewRecorder()
	srv.router.ServeHTTP(wEmpty, reqEmpty)
	if wEmpty.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty name, got %d", wEmpty.Code)
	}

	// 4. Successful rename via POST -> 200
	goodBody, _ := json.Marshal(map[string]string{"new_name": "test-ren"})
	reqGood := httptest.NewRequest("POST", "/api/nodes/test-orig/rename", bytes.NewReader(goodBody))
	reqGood.AddCookie(cookie)
	wGood := httptest.NewRecorder()
	srv.router.ServeHTTP(wGood, reqGood)
	if wGood.Code != http.StatusOK {
		t.Fatalf("Rename POST failed: %d %s", wGood.Code, wGood.Body.String())
	}

	var renNode config.Node
	if err := json.Unmarshal(wGood.Body.Bytes(), &renNode); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if renNode.Name != "test-ren" {
		t.Errorf("Expected node name 'test-ren', got %q", renNode.Name)
	}

	// 5. Successful rename via PUT
	putBody, _ := json.Marshal(map[string]string{"new_name": "test-final"})
	reqPut := httptest.NewRequest("PUT", "/api/nodes/test-ren/rename", bytes.NewReader(putBody))
	reqPut.AddCookie(cookie)
	wPut := httptest.NewRecorder()
	srv.router.ServeHTTP(wPut, reqPut)
	if wPut.Code != http.StatusOK {
		t.Fatalf("Rename PUT failed: %d %s", wPut.Code, wPut.Body.String())
	}
}

func TestNetworkPoliciesAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	cookie := w.Result().Cookies()[0]

	// 1. GET /api/network-policies (initially 3 built-in policies)
	reqGet := httptest.NewRequest("GET", "/api/network-policies", nil)
	reqGet.AddCookie(cookie)
	wGet := httptest.NewRecorder()
	srv.router.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("Get policies failed: %d %s", wGet.Code, wGet.Body.String())
	}
	var policies []config.NetworkPolicy
	_ = json.Unmarshal(wGet.Body.Bytes(), &policies)
	if len(policies) != 3 {
		t.Fatalf("Expected 3 initial policies, got %d", len(policies))
	}

	// 2. POST /api/network-policies with reserved ID -> 400
	reservedBody, _ := json.Marshal(config.NetworkPolicy{ID: "default", Name: "Hacked Default"})
	reqReserved := httptest.NewRequest("POST", "/api/network-policies", bytes.NewReader(reservedBody))
	reqReserved.AddCookie(cookie)
	wReserved := httptest.NewRecorder()
	srv.router.ServeHTTP(wReserved, reqReserved)
	if wReserved.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for reserved ID, got %d", wReserved.Code)
	}

	// 3. POST /api/network-policies with custom policy -> 201
	custPref := 140
	customPol := config.NetworkPolicy{
		ID:              "guest-net",
		Name:            "Guest Network",
		Description:     "Guest DMZ",
		AllowedDstCIDRs: []string{"172.20.10.0/24"},
		FilterForward:   true,
		Fwmark:          "51820",
		Preference:      &custPref,
		Mark:            "0x1234",
	}
	customBody, _ := json.Marshal(customPol)
	reqCustom := httptest.NewRequest("POST", "/api/network-policies", bytes.NewReader(customBody))
	reqCustom.AddCookie(cookie)
	wCustom := httptest.NewRecorder()
	srv.router.ServeHTTP(wCustom, reqCustom)
	if wCustom.Code != http.StatusCreated {
		t.Fatalf("Create policy failed: %d %s", wCustom.Code, wCustom.Body.String())
	}

	// 4. GET /api/network-policies/guest-net -> 200
	reqGetSingle := httptest.NewRequest("GET", "/api/network-policies/guest-net", nil)
	reqGetSingle.AddCookie(cookie)
	wGetSingle := httptest.NewRecorder()
	srv.router.ServeHTTP(wGetSingle, reqGetSingle)
	if wGetSingle.Code != http.StatusOK {
		t.Fatalf("Get policy failed: %d %s", wGetSingle.Code, wGetSingle.Body.String())
	}
	var fetchedPol config.NetworkPolicy
	_ = json.Unmarshal(wGetSingle.Body.Bytes(), &fetchedPol)
	if fetchedPol.Fwmark != "51820" || fetchedPol.Preference == nil || *fetchedPol.Preference != 140 || fetchedPol.Mark != "0x1234" {
		t.Errorf("Unexpected fetched policy: fwmark=%q, preference=%v, mark=%q", fetchedPol.Fwmark, fetchedPol.Preference, fetchedPol.Mark)
	}

	// 5. PUT /api/network-policies/guest-net -> 200
	updPref := 160
	updatePol := config.NetworkPolicy{
		Name:            "Updated Guest Network",
		AllowedDstCIDRs: []string{"172.20.15.0/24"},
		Fwmark:          "0xca64",
		Preference:      &updPref,
		Mark:            "0x5678",
	}
	updateBody, _ := json.Marshal(updatePol)
	reqUpdate := httptest.NewRequest("PUT", "/api/network-policies/guest-net", bytes.NewReader(updateBody))
	reqUpdate.AddCookie(cookie)
	wUpdate := httptest.NewRecorder()
	srv.router.ServeHTTP(wUpdate, reqUpdate)
	if wUpdate.Code != http.StatusOK {
		t.Fatalf("Update policy failed: %d %s", wUpdate.Code, wUpdate.Body.String())
	}
	var updatedPol config.NetworkPolicy
	_ = json.Unmarshal(wUpdate.Body.Bytes(), &updatedPol)
	if updatedPol.Mark != "0x5678" {
		t.Errorf("Unexpected updated policy mark: %q", updatedPol.Mark)
	}

	// 6. PUT /api/network-policies/default -> 400 (cannot edit built-in)
	reqUpdateDef := httptest.NewRequest("PUT", "/api/network-policies/default", bytes.NewReader(updateBody))
	reqUpdateDef.AddCookie(cookie)
	wUpdateDef := httptest.NewRecorder()
	srv.router.ServeHTTP(wUpdateDef, reqUpdateDef)
	if wUpdateDef.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 when updating built-in policy, got %d", wUpdateDef.Code)
	}

	// 7. DELETE /api/network-policies/dn42 -> 400 (cannot delete built-in)
	reqDelBuiltin := httptest.NewRequest("DELETE", "/api/network-policies/dn42", nil)
	reqDelBuiltin.AddCookie(cookie)
	wDelBuiltin := httptest.NewRecorder()
	srv.router.ServeHTTP(wDelBuiltin, reqDelBuiltin)
	if wDelBuiltin.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 when deleting built-in policy, got %d", wDelBuiltin.Code)
	}

	// 8. DELETE /api/network-policies/guest-net -> 200
	reqDel := httptest.NewRequest("DELETE", "/api/network-policies/guest-net", nil)
	reqDel.AddCookie(cookie)
	wDel := httptest.NewRecorder()
	srv.router.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("Delete policy failed: %d %s", wDel.Code, wDel.Body.String())
	}

	// 9. GET /api/network-policies/guest-net -> 404
	reqGetDeleted := httptest.NewRequest("GET", "/api/network-policies/guest-net", nil)
	reqGetDeleted.AddCookie(cookie)
	wGetDeleted := httptest.NewRecorder()
	srv.router.ServeHTTP(wGetDeleted, reqGetDeleted)
	if wGetDeleted.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for deleted policy, got %d", wGetDeleted.Code)
	}
}

func TestUpdateStateSpecifiedNodeAndObservability(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	reqLogin := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	wLogin := httptest.NewRecorder()
	srv.router.ServeHTTP(wLogin, reqLogin)
	cookie := wLogin.Result().Cookies()[0]

	// Add 2 nodes: node1 (offline host) and extNode (external)
	n1 := config.Node{Name: "offnode", Host: "127.0.0.1:59999", IP: "172.20.1.1", Interface: "eth0", ASN: 4224420001}
	b1, _ := json.Marshal(n1)
	req1 := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(b1))
	req1.AddCookie(cookie)
	w1 := httptest.NewRecorder()
	srv.router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("AddNode 1 failed: %d %s", w1.Code, w1.Body.String())
	}

	n2 := config.Node{Name: "extnode", Host: "none", IP: "172.20.1.2", IsExternal: true, ASN: 4224420002}
	b2, _ := json.Marshal(n2)
	req2 := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(b2))
	req2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	srv.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("AddNode 2 failed: %d %s", w2.Code, w2.Body.String())
	}

	// 1. Test POST /api/nodes/extnode/state (specific external node state update)
	reqExtState := httptest.NewRequest("POST", "/api/nodes/extnode/state", nil)
	reqExtState.AddCookie(cookie)
	wExtState := httptest.NewRecorder()
	srv.router.ServeHTTP(wExtState, reqExtState)
	if wExtState.Code != http.StatusOK {
		t.Fatalf("Update specific node state failed: %d %s", wExtState.Code, wExtState.Body.String())
	}

	var extResp map[string]any
	if err := json.Unmarshal(wExtState.Body.Bytes(), &extResp); err != nil {
		t.Fatalf("Failed to parse json: %v", err)
	}
	if extResp["success"] != true {
		t.Errorf("Expected success true, got %v", extResp["success"])
	}

	// 2. Test POST /api/state/update?node=offnode (specific offline node state update)
	// It should complete quickly and report offnode in failed_nodes and warnings
	reqOfflineState := httptest.NewRequest("POST", "/api/state/update?node=offnode", nil)
	reqOfflineState.AddCookie(cookie)
	wOfflineState := httptest.NewRecorder()
	srv.router.ServeHTTP(wOfflineState, reqOfflineState)
	if wOfflineState.Code != http.StatusOK {
		t.Fatalf("Update offline node state failed: %d %s", wOfflineState.Code, wOfflineState.Body.String())
	}

	var offlineResp struct {
		Success     bool              `json:"success"`
		FailedNodes map[string]string `json:"failed_nodes"`
		Warnings    []string          `json:"warnings"`
	}
	if err := json.Unmarshal(wOfflineState.Body.Bytes(), &offlineResp); err != nil {
		t.Fatalf("Failed to parse json: %v", err)
	}
	if !offlineResp.Success {
		t.Errorf("Expected success true even with failed node warning")
	}
	if offlineResp.FailedNodes["offnode"] == "" {
		t.Errorf("Expected offnode to be listed in failed_nodes, got %+v", offlineResp.FailedNodes)
	}
	if len(offlineResp.Warnings) == 0 {
		t.Errorf("Expected warning for offnode connection failure")
	}

	// 3. Delete offnode
	reqDel := httptest.NewRequest("DELETE", "/api/nodes/offnode", nil)
	reqDel.AddCookie(cookie)
	wDel := httptest.NewRecorder()
	srv.router.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("DeleteNode failed: %d %s", wDel.Code, wDel.Body.String())
	}

	// 4. Verify GET /api/nodes/status does not contain deleted node
	reqStatus := httptest.NewRequest("GET", "/api/nodes/status", nil)
	reqStatus.AddCookie(cookie)
	wStatus := httptest.NewRecorder()
	srv.router.ServeHTTP(wStatus, reqStatus)
	var statuses map[string]config.NodeStatus
	if err := json.Unmarshal(wStatus.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("Failed to decode statuses: %v", err)
	}
	if _, exists := statuses["offnode"]; exists {
		t.Errorf("Deleted offnode still exists in /api/nodes/status")
	}

	// 5. Run full POST /api/state/update (like clicking Update State in UI)
	reqFullUpdate := httptest.NewRequest("POST", "/api/state/update", nil)
	reqFullUpdate.AddCookie(cookie)
	wFullUpdate := httptest.NewRecorder()
	srv.router.ServeHTTP(wFullUpdate, reqFullUpdate)
	if wFullUpdate.Code != http.StatusOK {
		t.Fatalf("Full UpdateState failed: %d %s", wFullUpdate.Code, wFullUpdate.Body.String())
	}

	var fullResp struct {
		Success     bool              `json:"success"`
		FailedNodes map[string]string `json:"failed_nodes"`
	}
	if err := json.Unmarshal(wFullUpdate.Body.Bytes(), &fullResp); err != nil {
		t.Fatalf("Failed to parse json: %v", err)
	}
	if len(fullResp.FailedNodes) != 0 {
		t.Errorf("Expected no failed nodes after deleting offnode, got %+v", fullResp.FailedNodes)
	}
}

func TestMultipleLinksAPI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-srv-multilinks-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize store: %v", err)
	}

	mgr := engine.NewManager(store)
	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Manager:    mgr,
	})

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": pass})
	reqLogin := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	wLogin := httptest.NewRecorder()
	srv.router.ServeHTTP(wLogin, reqLogin)
	cookie := wLogin.Result().Cookies()[0]

	// Add node1 and node2
	node1 := config.Node{
		Name:      "test-n1",
		Host:      "192.168.1.10",
		IP:        "192.168.100.10",
		Interface: "lo",
		ASN:       4224420010,
	}
	node2 := config.Node{
		Name:      "test-n2",
		Host:      "192.168.1.20",
		IP:        "192.168.100.20",
		Interface: "lo",
		ASN:       4224420020,
	}
	bodyN1, _ := json.Marshal(node1)
	reqN1 := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(bodyN1))
	reqN1.AddCookie(cookie)
	wN1 := httptest.NewRecorder()
	srv.router.ServeHTTP(wN1, reqN1)
	if wN1.Code != http.StatusCreated {
		t.Fatalf("Add node1 failed: %d %s", wN1.Code, wN1.Body.String())
	}

	bodyN2, _ := json.Marshal(node2)
	reqN2 := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(bodyN2))
	reqN2.AddCookie(cookie)
	wN2 := httptest.NewRecorder()
	srv.router.ServeHTTP(wN2, reqN2)
	if wN2.Code != http.StatusCreated {
		t.Fatalf("Add node2 failed: %d %s", wN2.Code, wN2.Body.String())
	}

	// Create Link 1
	linkReq1, _ := json.Marshal(map[string]string{
		"from_node": "test-n1",
		"to_node":   "test-n2",
	})
	reqL1 := httptest.NewRequest("POST", "/api/links", bytes.NewReader(linkReq1))
	reqL1.AddCookie(cookie)
	wL1 := httptest.NewRecorder()
	srv.router.ServeHTTP(wL1, reqL1)
	if wL1.Code != http.StatusCreated {
		t.Fatalf("Create link 1 failed: %d %s", wL1.Code, wL1.Body.String())
	}
	var l1 config.Link
	_ = json.Unmarshal(wL1.Body.Bytes(), &l1)
	if l1.From.Interface != "wg42test-n2" || l1.To.Interface != "wg42test-n1" {
		t.Errorf("Link 1 interfaces unexpected: %s / %s", l1.From.Interface, l1.To.Interface)
	}

	// Create Link 2 between same nodes
	linkReq2, _ := json.Marshal(map[string]string{
		"from_node": "test-n1",
		"to_node":   "test-n2",
	})
	reqL2 := httptest.NewRequest("POST", "/api/links", bytes.NewReader(linkReq2))
	reqL2.AddCookie(cookie)
	wL2 := httptest.NewRecorder()
	srv.router.ServeHTTP(wL2, reqL2)
	if wL2.Code != http.StatusCreated {
		t.Fatalf("Create link 2 failed: %d %s", wL2.Code, wL2.Body.String())
	}
	var l2 config.Link
	_ = json.Unmarshal(wL2.Body.Bytes(), &l2)
	if l2.From.Interface != "wg42test-n21" || l2.To.Interface != "wg42test-n11" {
		t.Errorf("Link 2 interfaces unexpected: %s / %s", l2.From.Interface, l2.To.Interface)
	}
	if l2.From.ListenPort != l1.From.ListenPort+1 || l2.To.ListenPort != l1.To.ListenPort+1 {
		t.Errorf("Link 2 ports not incremented: l1=%d/%d, l2=%d/%d",
			l1.From.ListenPort, l1.To.ListenPort, l2.From.ListenPort, l2.To.ListenPort)
	}

	// Delete Link 2 specifically by interface
	delURL := fmt.Sprintf("/api/links?from=test-n1&to=test-n2&interface=%s", l2.From.Interface)
	reqDel := httptest.NewRequest("DELETE", delURL, nil)
	reqDel.AddCookie(cookie)
	wDel := httptest.NewRecorder()
	srv.router.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("Delete link 2 failed: %d %s", wDel.Code, wDel.Body.String())
	}

	// Verify remaining links count is 1 and it is link 1
	reqLinks := httptest.NewRequest("GET", "/api/links", nil)
	reqLinks.AddCookie(cookie)
	wLinks := httptest.NewRecorder()
	srv.router.ServeHTTP(wLinks, reqLinks)
	var links []config.Link
	_ = json.Unmarshal(wLinks.Body.Bytes(), &links)
	if len(links) != 1 {
		t.Fatalf("Expected 1 remaining link, got %d", len(links))
	}
	if links[0].From.Interface != "wg42test-n2" {
		t.Errorf("Expected remaining link to be link 1, got interface %s", links[0].From.Interface)
	}
}

func TestBlocksPersistence(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// 1. Unauthenticated request to /api/blocks should fail
	reqUnauth := httptest.NewRequest("GET", "/api/blocks", nil)
	wUnauth := httptest.NewRecorder()
	srv.router.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for unauthenticated GET /api/blocks, got %d", wUnauth.Code)
	}

	// 2. Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	reqLogin := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	wLogin := httptest.NewRecorder()
	srv.router.ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Fatalf("Login failed: %d %s", wLogin.Code, wLogin.Body.String())
	}
	cookie := wLogin.Result().Cookies()[0]

	// 3. Authenticated GET /api/blocks should return empty list initially
	reqGet := httptest.NewRequest("GET", "/api/blocks", nil)
	reqGet.AddCookie(cookie)
	wGet := httptest.NewRecorder()
	srv.router.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("Expected 200 for GET /api/blocks, got %d: %s", wGet.Code, wGet.Body.String())
	}
	var blocks []config.Block
	if err := json.Unmarshal(wGet.Body.Bytes(), &blocks); err != nil {
		t.Fatalf("Failed to unmarshal blocks: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("Expected 0 blocks initially, got %d", len(blocks))
	}

	// 4. Add two nodes: nodeA and nodeB
	for i, name := range []string{"nodeA", "nodeB"} {
		nodeObj := config.Node{Name: name, Host: "10.0.0.1", IP: fmt.Sprintf("192.168.1.%d", i+1)}
		b, _ := json.Marshal(nodeObj)
		reqNode := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(b))
		reqNode.AddCookie(cookie)
		wNode := httptest.NewRecorder()
		srv.router.ServeHTTP(wNode, reqNode)
		if wNode.Code != http.StatusCreated {
			t.Fatalf("Failed to create node %s: %d %s", name, wNode.Code, wNode.Body.String())
		}
	}

	// 5. Update blocks via PUT /api/blocks
	testBlocks := []config.Block{
		{
			ID:     "block-core",
			Name:   "Core Mesh",
			Color:  "#6366F1",
			X:      100,
			Y:      150,
			Width:  500,
			Height: 300,
			Nodes:  []string{"nodeA", "nodeB"},
		},
		{
			ID:     "block-edge",
			Name:   "Edge Block",
			Color:  "#10B981",
			X:      700,
			Y:      150,
			Width:  400,
			Height: 250,
			Nodes:  []string{},
		},
	}
	putPayload, _ := json.Marshal(testBlocks)
	reqPut := httptest.NewRequest("PUT", "/api/blocks", bytes.NewReader(putPayload))
	reqPut.AddCookie(cookie)
	wPut := httptest.NewRecorder()
	srv.router.ServeHTTP(wPut, reqPut)
	if wPut.Code != http.StatusOK {
		t.Fatalf("Expected 200 for PUT /api/blocks, got %d: %s", wPut.Code, wPut.Body.String())
	}

	// 6. Verify GET returns updated blocks
	reqGet2 := httptest.NewRequest("GET", "/api/blocks", nil)
	reqGet2.AddCookie(cookie)
	wGet2 := httptest.NewRecorder()
	srv.router.ServeHTTP(wGet2, reqGet2)
	var gotBlocks []config.Block
	_ = json.Unmarshal(wGet2.Body.Bytes(), &gotBlocks)
	if len(gotBlocks) != 2 {
		t.Fatalf("Expected 2 blocks, got %d", len(gotBlocks))
	}
	if gotBlocks[0].ID != "block-core" || len(gotBlocks[0].Nodes) != 2 {
		t.Errorf("Unexpected block-core: %+v", gotBlocks[0])
	}

	// 7. Rename nodeA -> nodeA-renamed, verify block-core nodes updated
	renamePayload, _ := json.Marshal(map[string]string{"new_name": "nodeA-new"})
	reqRename := httptest.NewRequest("POST", "/api/nodes/nodeA/rename", bytes.NewReader(renamePayload))
	reqRename.AddCookie(cookie)
	wRename := httptest.NewRecorder()
	srv.router.ServeHTTP(wRename, reqRename)
	if wRename.Code != http.StatusOK {
		t.Fatalf("Rename failed: %d %s", wRename.Code, wRename.Body.String())
	}

	reqGet3 := httptest.NewRequest("GET", "/api/blocks", nil)
	reqGet3.AddCookie(cookie)
	wGet3 := httptest.NewRecorder()
	srv.router.ServeHTTP(wGet3, reqGet3)
	var gotBlocksRename []config.Block
	_ = json.Unmarshal(wGet3.Body.Bytes(), &gotBlocksRename)
	if len(gotBlocksRename[0].Nodes) != 2 || gotBlocksRename[0].Nodes[0] != "nodeA-new" {
		t.Errorf("Expected nodeA-new in block-core, got: %v", gotBlocksRename[0].Nodes)
	}

	// 8. Delete nodeB, verify block-core nodes updated
	reqDel := httptest.NewRequest("DELETE", "/api/nodes/nodeB", nil)
	reqDel.AddCookie(cookie)
	wDel := httptest.NewRecorder()
	srv.router.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("Delete failed: %d %s", wDel.Code, wDel.Body.String())
	}

	reqGet4 := httptest.NewRequest("GET", "/api/blocks", nil)
	reqGet4.AddCookie(cookie)
	wGet4 := httptest.NewRecorder()
	srv.router.ServeHTTP(wGet4, reqGet4)
	var gotBlocksDel []config.Block
	_ = json.Unmarshal(wGet4.Body.Bytes(), &gotBlocksDel)
	if len(gotBlocksDel[0].Nodes) != 1 || gotBlocksDel[0].Nodes[0] != "nodeA-new" {
		t.Errorf("Expected only nodeA-new in block-core after deleting nodeB, got: %v", gotBlocksDel[0].Nodes)
	}

	// 9. Recreate server instance from same tempDir to verify config.json persistence on disk
	storeReload := config.NewStore(tempDir)
	mgrReload := engine.NewManager(storeReload)
	srvReload := New(Config{
		ListenAddr: "127.0.0.1:0",
		Manager:    mgrReload,
	})
	reqReload := httptest.NewRequest("GET", "/api/blocks", nil)
	reqReload.AddCookie(cookie)
	wReload := httptest.NewRecorder()
	srvReload.router.ServeHTTP(wReload, reqReload)
	if wReload.Code != http.StatusOK {
		t.Fatalf("Expected 200 on reloaded server, got %d", wReload.Code)
	}
	var gotBlocksReload []config.Block
	_ = json.Unmarshal(wReload.Body.Bytes(), &gotBlocksReload)
	if len(gotBlocksReload) != 2 {
		t.Fatalf("Expected 2 blocks on disk reload, got %d", len(gotBlocksReload))
	}
	if gotBlocksReload[0].ID != "block-core" || gotBlocksReload[0].Name != "Core Mesh" {
		t.Errorf("Unexpected reloaded block data: %+v", gotBlocksReload[0])
	}
}

func TestRoutingPolicyServerAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login to get session cookie
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	reqLogin := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	wLogin := httptest.NewRecorder()
	srv.router.ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Fatalf("Login failed: %d", wLogin.Code)
	}
	cookie := wLogin.Result().Cookies()[0]

	// 1. Create custom NetworkPolicy with routing_policy
	polPayload := map[string]interface{}{
		"id":             "pol-custom-stub",
		"name":           "Custom Stub Policy",
		"routing_policy": "stub",
	}
	polBody, _ := json.Marshal(polPayload)
	reqPol := httptest.NewRequest("POST", "/api/network-policies", bytes.NewReader(polBody))
	reqPol.AddCookie(cookie)
	wPol := httptest.NewRecorder()
	srv.router.ServeHTTP(wPol, reqPol)
	if wPol.Code != http.StatusCreated {
		t.Fatalf("Create policy failed: %d %s", wPol.Code, wPol.Body.String())
	}

	var createdPol config.NetworkPolicy
	_ = json.Unmarshal(wPol.Body.Bytes(), &createdPol)
	if createdPol.RoutingPolicy != "stub" {
		t.Errorf("Expected policy RoutingPolicy 'stub', got %q", createdPol.RoutingPolicy)
	}

	// 2. Add two nodes
	n1Body, _ := json.Marshal(config.Node{Name: "n-rt1", Host: "h1", IP: "192.168.1.1", Interface: "eth0", ASN: 4224420001})
	reqN1 := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(n1Body))
	reqN1.AddCookie(cookie)
	wN1 := httptest.NewRecorder()
	srv.router.ServeHTTP(wN1, reqN1)
	if wN1.Code != http.StatusCreated {
		t.Fatalf("AddNode 1 failed: %d", wN1.Code)
	}

	n2Body, _ := json.Marshal(config.Node{Name: "n-rt2", Host: "h2", IP: "192.168.1.2", Interface: "eth0", ASN: 4224420002})
	reqN2 := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(n2Body))
	reqN2.AddCookie(cookie)
	wN2 := httptest.NewRecorder()
	srv.router.ServeHTTP(wN2, reqN2)
	if wN2.Code != http.StatusCreated {
		t.Fatalf("AddNode 2 failed: %d", wN2.Code)
	}

	// 3. Add link with from_routing_policy
	linkPayload := map[string]interface{}{
		"from_node":           "n-rt1",
		"to_node":             "n-rt2",
		"from_routing_policy": "stub",
	}
	linkBody, _ := json.Marshal(linkPayload)
	reqLink := httptest.NewRequest("POST", "/api/links", bytes.NewReader(linkBody))
	reqLink.AddCookie(cookie)
	wLink := httptest.NewRecorder()
	srv.router.ServeHTTP(wLink, reqLink)
	if wLink.Code != http.StatusCreated {
		t.Fatalf("AddLink failed: %d %s", wLink.Code, wLink.Body.String())
	}

	var createdLink config.Link
	_ = json.Unmarshal(wLink.Body.Bytes(), &createdLink)
	var end1 *config.LinkEnd
	if createdLink.From.Name == "n-rt1" {
		end1 = &createdLink.From
	} else {
		end1 = &createdLink.To
	}
	if end1.RoutingPolicy != "stub" {
		t.Errorf("Expected link end routing_policy 'stub', got %q", end1.RoutingPolicy)
	}

	// 4. Update link with from_routing_policy: receive_only
	updatePayload := map[string]interface{}{
		"from_node":           "n-rt1",
		"to_node":             "n-rt2",
		"from_routing_policy": "receive_only",
	}
	updateBody, _ := json.Marshal(updatePayload)
	reqUpdate := httptest.NewRequest("PUT", "/api/links", bytes.NewReader(updateBody))
	reqUpdate.AddCookie(cookie)
	wUpdate := httptest.NewRecorder()
	srv.router.ServeHTTP(wUpdate, reqUpdate)
	if wUpdate.Code != http.StatusOK {
		t.Fatalf("UpdateLink failed: %d %s", wUpdate.Code, wUpdate.Body.String())
	}

	var updatedLink config.Link
	_ = json.Unmarshal(wUpdate.Body.Bytes(), &updatedLink)
	if updatedLink.From.Name == "n-rt1" {
		end1 = &updatedLink.From
	} else {
		end1 = &updatedLink.To
	}
	if end1.RoutingPolicy != "receive_only" {
		t.Errorf("Expected updated link end routing_policy 'receive_only', got %q", end1.RoutingPolicy)
	}
}

func TestRestartEndpoints(t *testing.T) {
	srv, tmpDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	cookie := w.Result().Cookies()[0]

	// Add an external node
	extNode := config.Node{
		Name:       "ext-node",
		IsExternal: true,
		ASN:        4242421000,
	}
	bodyNode, _ := json.Marshal(extNode)
	reqNode := httptest.NewRequest("POST", "/api/nodes", bytes.NewReader(bodyNode))
	reqNode.AddCookie(cookie)
	wNode := httptest.NewRecorder()
	srv.router.ServeHTTP(wNode, reqNode)
	if wNode.Code != http.StatusCreated {
		t.Fatalf("AddNode failed: %d %s", wNode.Code, wNode.Body.String())
	}

	// 1. Test POST /api/nodes/nonexistent/restart-wg -> error
	reqRestartNonexistent := httptest.NewRequest("POST", "/api/nodes/nonexistent/restart-wg", nil)
	reqRestartNonexistent.AddCookie(cookie)
	wRestartNonexistent := httptest.NewRecorder()
	srv.router.ServeHTTP(wRestartNonexistent, reqRestartNonexistent)
	if wRestartNonexistent.Code == http.StatusOK {
		t.Fatalf("Expected error for nonexistent node restart-wg, got 200")
	}

	// 2. Test POST /api/nodes/ext-node/restart-wg -> error (external node cannot be SSH managed)
	reqRestartExt := httptest.NewRequest("POST", "/api/nodes/ext-node/restart-wg", nil)
	reqRestartExt.AddCookie(cookie)
	wRestartExt := httptest.NewRecorder()
	srv.router.ServeHTTP(wRestartExt, reqRestartExt)
	if wRestartExt.Code == http.StatusOK {
		t.Fatalf("Expected error for external node restart-wg, got 200")
	}

	// 3. Test POST /api/nodes/ext-node/restart-bird -> error
	reqRestartBird := httptest.NewRequest("POST", "/api/nodes/ext-node/restart-bird", nil)
	reqRestartBird.AddCookie(cookie)
	wRestartBird := httptest.NewRecorder()
	srv.router.ServeHTTP(wRestartBird, reqRestartBird)
	if wRestartBird.Code == http.StatusOK {
		t.Fatalf("Expected error for external node restart-bird, got 200")
	}

	// 4. Test POST /api/nodes/ext-node/interfaces/wg42001/restart -> error
	reqRestartIface := httptest.NewRequest("POST", "/api/nodes/ext-node/interfaces/wg42001/restart", nil)
	reqRestartIface.AddCookie(cookie)
	wRestartIface := httptest.NewRecorder()
	srv.router.ServeHTTP(wRestartIface, reqRestartIface)
	if wRestartIface.Code == http.StatusOK {
		t.Fatalf("Expected error for external node interface restart, got 200")
	}

	// 5. Test POST /api/nodes/ext-node/restart-wg/wg42001 -> error (alias)
	reqRestartIfaceAlias := httptest.NewRequest("POST", "/api/nodes/ext-node/restart-wg/wg42001", nil)
	reqRestartIfaceAlias.AddCookie(cookie)
	wRestartIfaceAlias := httptest.NewRecorder()
	srv.router.ServeHTTP(wRestartIfaceAlias, reqRestartIfaceAlias)
	if wRestartIfaceAlias.Code == http.StatusOK {
		t.Fatalf("Expected error for external node interface restart alias, got 200")
	}
}




