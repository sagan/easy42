package config

import (
	"reflect"
	"testing"
)

func TestExtractNoteData(t *testing.T) {
	tests := []struct {
		name     string
		note     string
		expected map[string]any
	}{
		{
			name:     "empty note",
			note:     "",
			expected: map[string]any{},
		},
		{
			name:     "no code fence",
			note:     "# Just a markdown note\nSome text.",
			expected: map[string]any{},
		},
		{
			name: "json block standard",
			note: "```json\n{\n  \"type\": \"server\"\n}\n```",
			expected: map[string]any{
				"type": "server",
			},
		},
		{
			name: "json block with uppercase JSON",
			note: "```JSON\n{\n  \"type\": \"server\",\n  \"port\": 8080\n}\n```",
			expected: map[string]any{
				"type": "server",
				"port": float64(8080),
			},
		},
		{
			name: "block without language tag",
			note: "```\n{\n  \"type\": \"gateway\",\n  \"enabled\": true\n}\n```",
			expected: map[string]any{
				"type":    "gateway",
				"enabled": true,
			},
		},
		{
			name: "block with surrounding text",
			note: "# Server Header\nDescription here.\n\n```json\n{\n  \"type\": \"server\",\n  \"nested\": {\"val\": 42}\n}\n```\n\nFooter note.",
			expected: map[string]any{
				"type": "server",
				"nested": map[string]any{
					"val": float64(42),
				},
			},
		},
		{
			name: "first block is bash, second is json",
			note: "```bash\necho hello\n```\n\n```json\n{\n  \"type\": \"server\"\n}\n```",
			expected: map[string]any{
				"type": "server",
			},
		},
		{
			name:     "invalid json ignored",
			note:     "```json\n{ invalid json\n```",
			expected: map[string]any{},
		},
		{
			name:     "empty block",
			note:     "```json\n```",
			expected: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractNoteData(tt.note)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("ExtractNoteData() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNodeLinkLinkEndDataMethods(t *testing.T) {
	node := &Node{
		Note: "```json\n{\"role\": \"core\"}\n```",
	}
	if node.Data()["role"] != "core" {
		t.Errorf("expected node.Data()[role] == core, got %v", node.Data()["role"])
	}

	link := &Link{
		Note: "```\n{\"bandwidth\": \"10G\"}\n```",
	}
	if link.Data()["bandwidth"] != "10G" {
		t.Errorf("expected link.Data()[bandwidth] == 10G, got %v", link.Data()["bandwidth"])
	}

	end := &LinkEnd{
		Note: "```json\n{\"mtu_override\": 1400}\n```",
	}
	if end.Data()["mtu_override"] != float64(1400) {
		t.Errorf("expected end.Data()[mtu_override] == 1400, got %v", end.Data()["mtu_override"])
	}

	var nilNode *Node
	if len(nilNode.Data()) != 0 {
		t.Errorf("expected nilNode.Data() to be empty map")
	}
}
