package compiler

import (
	"strings"
	"testing"

	"easy42/internal/config"
)

func TestBuiltinTemplates(t *testing.T) {
	builtins := GetBuiltinTemplates()
	if len(builtins) != 3 {
		t.Fatalf("expected 3 builtin templates, got %d", len(builtins))
	}

	typesFound := make(map[string]bool)
	for _, tmpl := range builtins {
		if !tmpl.IsBuiltin {
			t.Errorf("template %s should be marked IsBuiltin", tmpl.ID)
		}
		if tmpl.Content == "" {
			t.Errorf("template %s content should not be empty", tmpl.ID)
		}
		typesFound[tmpl.Type] = true
	}

	for _, expectedType := range []string{config.TemplateTypeWg, config.TemplateTypeBird, config.TemplateTypeNft} {
		if !typesFound[expectedType] {
			t.Errorf("expected builtin template for type %s", expectedType)
		}
	}
}

func TestValidateTemplate(t *testing.T) {
	// Valid templates
	if err := ValidateTemplate(config.TemplateTypeBird, "# BIRD template\nrouter id {{ .ip }};\n"); err != nil {
		t.Errorf("valid bird template failed: %v", err)
	}
	if err := ValidateTemplate(config.TemplateTypeWg, "[Interface]\nPrivateKey = {{ .self.private_key }}\n"); err != nil {
		t.Errorf("valid wg template failed: %v", err)
	}
	if err := ValidateTemplate(config.TemplateTypeNft, "table inet easy42 {\n}\n"); err != nil {
		t.Errorf("valid nft template failed: %v", err)
	}

	// Invalid type
	if err := ValidateTemplate("invalid_type", "test"); err == nil {
		t.Errorf("expected error for invalid type")
	}

	// Empty content
	if err := ValidateTemplate(config.TemplateTypeBird, "   "); err == nil {
		t.Errorf("expected error for empty content")
	}

	// Syntax error
	if err := ValidateTemplate(config.TemplateTypeBird, "router id {{ .invalid syntax"); err == nil {
		t.Errorf("expected syntax error for unclosed action")
	}
}

func TestFindTemplate(t *testing.T) {
	customTemplates := []config.ConfigTemplate{
		{
			ID:      "my-custom-bird",
			Name:    "Custom Bird Template",
			Type:    config.TemplateTypeBird,
			Content: "# Custom Bird Config for {{ .name }}",
		},
		{
			ID:      "my-custom-wg",
			Name:    "Custom WG Template",
			Type:    config.TemplateTypeWg,
			Content: "# Custom WG Config for {{ .self_node.name }}",
		},
	}

	// 1. Empty or "default" returns built-in
	content, err := FindTemplate(config.TemplateTypeBird, "", customTemplates)
	if err != nil || !strings.Contains(content, "protocol bgp") {
		t.Errorf("expected default bird template, got err: %v", err)
	}
	content, err = FindTemplate(config.TemplateTypeBird, "default", customTemplates)
	if err != nil || !strings.Contains(content, "protocol bgp") {
		t.Errorf("expected default bird template, got err: %v", err)
	}

	// 2. Find by custom ID
	content, err = FindTemplate(config.TemplateTypeBird, "my-custom-bird", customTemplates)
	if err != nil || content != "# Custom Bird Config for {{ .name }}" {
		t.Errorf("expected custom bird content, got: %s (err: %v)", content, err)
	}

	// 3. Find by custom Name
	content, err = FindTemplate(config.TemplateTypeBird, "Custom Bird Template", customTemplates)
	if err != nil || content != "# Custom Bird Config for {{ .name }}" {
		t.Errorf("expected custom bird content by name, got: %s (err: %v)", content, err)
	}

	// 4. Find built-in by ID
	content, err = FindTemplate(config.TemplateTypeWg, config.DefaultWgTemplateID, customTemplates)
	if err != nil || !strings.Contains(content, "[Interface]") {
		t.Errorf("expected default wg template by ID, got err: %v", err)
	}

	// 5. Not found
	_, err = FindTemplate(config.TemplateTypeBird, "non-existent", customTemplates)
	if err == nil {
		t.Errorf("expected error for non-existent template")
	}
}

func TestCustomTemplatesInConfigGeneration(t *testing.T) {
	node := &config.Node{
		Name:         "testnode",
		IP:           "192.168.100.1",
		ASN:          4224420001,
		BirdTemplate: "custom-bird",
		NftTemplate:  "custom-nft",
		WgTemplate:   "custom-wg",
	}

	customBirdTmpl := config.ConfigTemplate{
		ID:      "custom-bird",
		Name:    "Custom Bird",
		Type:    config.TemplateTypeBird,
		Content: "/* CUSTOM_BIRD_HEADER */\nrouter id {{ .ip }};\n",
	}

	customNftTmpl := config.ConfigTemplate{
		ID:      "custom-nft",
		Name:    "Custom NFT",
		Type:    config.TemplateTypeNft,
		Content: "# CUSTOM_NFT_HEADER\ntable inet easy42 {\n}\n",
	}

	customWgTmpl := config.ConfigTemplate{
		ID:      "custom-wg",
		Name:    "Custom WG",
		Type:    config.TemplateTypeWg,
		Content: "# CUSTOM_WG_HEADER\n[Interface]\nAddress = {{ .address }}\n",
	}

	cfg := &config.Config{
		Nodes:     []config.Node{*node},
		Templates: []config.ConfigTemplate{customBirdTmpl, customNftTmpl, customWgTmpl},
	}

	// Test Bird config generation uses custom template
	birdConf, err := GenerateBirdConfig(node, cfg.Nodes, nil, cfg)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}
	if !strings.Contains(birdConf, "CUSTOM_BIRD_HEADER") {
		t.Errorf("expected bird config to contain CUSTOM_BIRD_HEADER, got:\n%s", birdConf)
	}
	if !strings.Contains(birdConf, "router id 192.168.100.1;") {
		t.Errorf("expected bird config to contain router id, got:\n%s", birdConf)
	}

	// Test Nftables config generation uses custom template
	nftConf, err := GenerateNftablesConfig(node, cfg.Nodes, nil, cfg)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}
	if !strings.Contains(nftConf, "CUSTOM_NFT_HEADER") {
		t.Errorf("expected nft config to contain CUSTOM_NFT_HEADER, got:\n%s", nftConf)
	}

	// Test WireGuard config generation uses custom template
	peerNode := &config.Node{Name: "peer", IP: "192.168.100.2", ASN: 4224420002}
	selfEnd := &config.LinkEnd{Name: "testnode", Address: "fe80::1/64"}
	peerEnd := &config.LinkEnd{Name: "peer", Address: "fe80::2/64"}

	wgConf, err := GenerateWgConfigContent(node, peerNode, selfEnd, peerEnd, nil, cfg)
	if err != nil {
		t.Fatalf("GenerateWgConfigContent failed: %v", err)
	}
	if !strings.Contains(wgConf, "CUSTOM_WG_HEADER") {
		t.Errorf("expected wg config to contain CUSTOM_WG_HEADER, got:\n%s", wgConf)
	}
	if !strings.Contains(wgConf, "Address = fe80::1/64") {
		t.Errorf("expected wg config to contain Address = fe80::1/64, got:\n%s", wgConf)
	}
}

func TestNoteDataInCustomTemplates(t *testing.T) {
	nodeA := &config.Node{
		Name: "node-a",
		IP:   "192.168.100.1",
		ASN:  4224420001,
		Note: "# Node A Markdown\n```json\n{\n  \"type\": \"server\",\n  \"zone\": \"eu-west\"\n}\n```",
	}

	nodeB := &config.Node{
		Name: "node-b",
		IP:   "192.168.100.2",
		ASN:  4224420002,
		Note: "```json\n{\n  \"type\": \"client\",\n  \"role\": \"worker\"\n}\n```",
	}

	endA := &config.LinkEnd{
		Name:      "node-a",
		Interface: "wg42nodeb",
		Address:   "fe80::1/64",
		Note:      "```json\n{\n  \"mtu\": 1420,\n  \"local_tag\": \"primary\"\n}\n```",
	}

	endB := &config.LinkEnd{
		Name:      "node-b",
		Interface: "wg42nodea",
		Address:   "fe80::2/64",
		Note:      "```\n{\n  \"remote_tag\": \"backup\"\n}\n```",
	}

	link := config.Link{
		From: *endA,
		To:   *endB,
		Note: "```json\n{\n  \"carrier\": \"fiber\",\n  \"cost\": 10\n}\n```",
	}

	allNodes := []config.Node{*nodeA, *nodeB}
	links := []config.Link{link}

	// 1. Test BIRD custom template
	birdTmpl := `# BIRD for {{ .name }}
# Node Type: {{ .data.type }}
# Node Zone: {{ .data.zone }}
{{ range .links }}
# Link Carrier: {{ .data.carrier }}
# Link Carrier via link: {{ .link.data.carrier }}
# Local Tag: {{ .local.data.local_tag }}
# Remote Tag: {{ .remote.data.remote_tag }}
# Remote Node Role: {{ .remote_node.data.role }}
{{ end }}
`
	birdConf, err := GenerateBirdConfigWithTemplate(birdTmpl, nodeA, allNodes, links)
	if err != nil {
		t.Fatalf("GenerateBirdConfigWithTemplate failed: %v", err)
	}
	if !strings.Contains(birdConf, "# Node Type: server") {
		t.Errorf("expected '# Node Type: server' in bird config, got:\n%s", birdConf)
	}
	if !strings.Contains(birdConf, "# Node Zone: eu-west") {
		t.Errorf("expected '# Node Zone: eu-west' in bird config, got:\n%s", birdConf)
	}
	if !strings.Contains(birdConf, "# Link Carrier: fiber") {
		t.Errorf("expected '# Link Carrier: fiber' in bird config, got:\n%s", birdConf)
	}
	if !strings.Contains(birdConf, "# Link Carrier via link: fiber") {
		t.Errorf("expected '# Link Carrier via link: fiber' in bird config, got:\n%s", birdConf)
	}
	if !strings.Contains(birdConf, "# Local Tag: primary") {
		t.Errorf("expected '# Local Tag: primary' in bird config, got:\n%s", birdConf)
	}
	if !strings.Contains(birdConf, "# Remote Tag: backup") {
		t.Errorf("expected '# Remote Tag: backup' in bird config, got:\n%s", birdConf)
	}
	if !strings.Contains(birdConf, "# Remote Node Role: worker") {
		t.Errorf("expected '# Remote Node Role: worker' in bird config, got:\n%s", birdConf)
	}

	// 2. Test WireGuard custom template
	wgTmpl := `[Interface]
# Node Type: {{ .data.type }}
# Self Node Type: {{ .self_node.data.type }}
# Peer Node Type: {{ .peer_node.data.type }}
# Self End Local Tag: {{ .self_end.data.local_tag }}
# Peer End Remote Tag: {{ .peer_end.data.remote_tag }}
# Link Carrier: {{ .link.data.carrier }}
`
	fullCfg := &config.Config{
		Nodes: allNodes,
		Links: links,
	}
	wgConf, err := GenerateWgConfigContentWithTemplate(wgTmpl, nodeA, nodeB, endA, endB, nil, fullCfg)
	if err != nil {
		t.Fatalf("GenerateWgConfigContentWithTemplate failed: %v", err)
	}
	if !strings.Contains(wgConf, "# Node Type: server") {
		t.Errorf("expected '# Node Type: server' in wg config, got:\n%s", wgConf)
	}
	if !strings.Contains(wgConf, "# Self Node Type: server") {
		t.Errorf("expected '# Self Node Type: server' in wg config, got:\n%s", wgConf)
	}
	if !strings.Contains(wgConf, "# Peer Node Type: client") {
		t.Errorf("expected '# Peer Node Type: client' in wg config, got:\n%s", wgConf)
	}
	if !strings.Contains(wgConf, "# Self End Local Tag: primary") {
		t.Errorf("expected '# Self End Local Tag: primary' in wg config, got:\n%s", wgConf)
	}
	if !strings.Contains(wgConf, "# Peer End Remote Tag: backup") {
		t.Errorf("expected '# Peer End Remote Tag: backup' in wg config, got:\n%s", wgConf)
	}
	if !strings.Contains(wgConf, "# Link Carrier: fiber") {
		t.Errorf("expected '# Link Carrier: fiber' in wg config, got:\n%s", wgConf)
	}

	// 3. Test nftables custom template
	nftTmpl := `table inet easy42 {
	# Node Type: {{ .data.type }}
}
`
	nftConf, err := GenerateNftablesConfigWithTemplate(nftTmpl, nodeA, allNodes, links)
	if err != nil {
		t.Fatalf("GenerateNftablesConfigWithTemplate failed: %v", err)
	}
	if !strings.Contains(nftConf, "# Node Type: server") {
		t.Errorf("expected '# Node Type: server' in nft config, got:\n%s", nftConf)
	}

	// 4. Test best-effort error tolerance (invalid JSON, no code block, empty Note)
	invalidNode := &config.Node{
		Name: "invalid-node",
		IP:   "192.168.100.99",
		Note: "```json\n{ not valid json\n```",
	}
	safeTmpl := `Node Type: [{{ if .data.type }}{{ .data.type }}{{ else }}empty{{ end }}]`
	safeBird, err := GenerateBirdConfigWithTemplate(safeTmpl, invalidNode, []config.Node{*invalidNode}, nil)
	if err != nil {
		t.Fatalf("expected invalid json to be ignored without error, got: %v", err)
	}
	if !strings.Contains(safeBird, "Node Type: [empty]") {
		t.Errorf("expected empty data for invalid json, got:\n%s", safeBird)
	}

	safeWg, err := GenerateWgConfigContentWithTemplate(safeTmpl, invalidNode, nodeB, endA, endB, nil)
	if err != nil {
		t.Fatalf("expected invalid json to be ignored in wg, got: %v", err)
	}
	if !strings.Contains(safeWg, "Node Type: [empty]") {
		t.Errorf("expected empty data in wg for invalid json, got:\n%s", safeWg)
	}

	safeNft, err := GenerateNftablesConfigWithTemplate(safeTmpl, invalidNode, []config.Node{*invalidNode}, nil)
	if err != nil {
		t.Fatalf("expected invalid json to be ignored in nft, got: %v", err)
	}
	if !strings.Contains(safeNft, "Node Type: [empty]") {
		t.Errorf("expected empty data in nft for invalid json, got:\n%s", safeNft)
	}
}
