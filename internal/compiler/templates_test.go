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
