package server

import (
	"bytes"
	"encoding/json"
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
	}
	bodyLink, _ := json.Marshal(linkReq)
	reqLink := httptest.NewRequest("POST", "/api/links", bytes.NewReader(bodyLink))
	reqLink.AddCookie(cookie)
	wLink := httptest.NewRecorder()
	srv.router.ServeHTTP(wLink, reqLink)
	if wLink.Code != http.StatusCreated {
		t.Fatalf("AddLink failed: %d %s", wLink.Code, wLink.Body.String())
	}

	// Update link
	updateReq := map[string]any{
		"from_node": "n1",
		"to_node":   "n2",
		"from_port": 51000,
		"to_port":   52000,
		"from_mtu":  1360,
		"to_mtu":    1360,
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
		PublicASN:      4242421234,
		ConfedMembers:  "4224420000..4224429999",
		ExportPrefixes: []string{"172.20.10.0/24"},
		ImportPrefixes: []string{"172.20.0.0/14{21,29}"},
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

