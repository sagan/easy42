package apps

import (
	"testing"

	"easy42/internal/config"
)

func TestInterpolateVariables(t *testing.T) {
	globalVars := map[string]string{
		"DOMAIN":      "easy42.net",
		"API_PORT":    "8080",
		"DB_PASSWORD": "supersecretpassword",
		"EMPTY_VAL":   "",
	}

	contextVars := map[string]string{
		"NODE_NAME":      "node-sg",
		"NODE_INTERFACE": "dummy0",
		"NODE_IP":        "10.42.0.1",
		"NODE_IP6":       "fd42:a159:f9f0::1",
		"APP_NAME":       "vproxy",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Direct dollar brace substitution",
			input:    "server_name: ${DOMAIN}\nlisten: ${API_PORT}",
			expected: "server_name: easy42.net\nlisten: 8080",
		},
		{
			name:     "Mustache substitution",
			input:    "Host: {{DOMAIN}}, IP: {{NODE_IP}}, Dot: {{.NODE_NAME}}",
			expected: "Host: easy42.net, IP: 10.42.0.1, Dot: node-sg",
		},
		{
			name:     "Default value fallback when missing",
			input:    "timeout: ${UNDEFINED_TIMEOUT:-30s}",
			expected: "timeout: 30s",
		},
		{
			name:     "Default value fallback when empty",
			input:    "val: ${EMPTY_VAL:-defaultval}",
			expected: "val: defaultval",
		},
		{
			name:     "Preserve unknown without default",
			input:    "image: nginx:${UNKNOWN_TAG}",
			expected: "image: nginx:${UNKNOWN_TAG}",
		},
		{
			name:     "Context vars override global vars",
			input:    "node is ${NODE_NAME}",
			expected: "node is node-sg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InterpolateVariables(tt.input, globalVars, contextVars)
			if got != tt.expected {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.expected)
			}
		})
	}
}

func TestBuildNodeContextVars(t *testing.T) {
	node := config.Node{
		Name:                "node-tokyo",
		Interface:           "dummy0",
		IP:                  "10.42.100.5/32",
		IP6:                 "fd42:a159:f9f0:1::5/128",
		ExternalIP:          "198.51.100.22/32",
		ExternalIP6:         "2001:db8:1::22/64",
		CustomNetworkScript: "#!/bin/sh\necho custom",
		Note: "```json\n{\n  \"cluster\": \"edge-asia\",\n  \"asn\": 4242421234,\n  \"meta\": {\n    \"env\": \"prod\",\n    \"tier\": 1\n  }\n}\n```",
	}

	ctx := BuildNodeContextVars(node)

	// Consistent with docs/device-helper.md:L153-L160
	expected := map[string]string{
		"NODE_NAME":         "node-tokyo",
		"NODE_INTERFACE":    "dummy0",
		"NODE_IP":           "10.42.100.5",
		"NODE_IP6":          "fd42:a159:f9f0:1::5",
		"NODE_EXTERNAL_IP":  "198.51.100.22",
		"NODE_EXTERNAL_IP6": "2001:db8:1::22",
		"HAS_CUSTOM_SCRIPT": "1",
		// Aliases
		"NODE_IPV4": "10.42.100.5",
		"NODE_IPV6": "fd42:a159:f9f0:1::5",
		// Layer 0 Note data
		"NODE_DATA_CLUSTER": "edge-asia",
		"NODE_DATA_ASN":     "4242421234",
		// Layer 1 Note data
		"NODE_DATA_META_ENV":  "prod",
		"NODE_DATA_META_TIER": "1",
		// Aliases without parent prefix
		"NODE_DATA_ENV":  "prod",
		"NODE_DATA_TIER": "1",
	}

	for k, expectedVal := range expected {
		actualVal, ok := ctx[k]
		if !ok {
			t.Errorf("missing expected context variable: %s", k)
		} else if actualVal != expectedVal {
			t.Errorf("variable %s: got %q, want %q", k, actualVal, expectedVal)
		}
	}

	// Verify that if ExternalIP == IP, it is not added as separate external IP
	sameIPNode := config.Node{
		Name:       "node-same",
		IP:         "10.42.1.1/32",
		ExternalIP: "10.42.1.1",
	}
	sameCtx := BuildNodeContextVars(sameIPNode)
	if _, ok := sameCtx["NODE_EXTERNAL_IP"]; ok {
		t.Errorf("NODE_EXTERNAL_IP should not be present when equal to NODE_IP")
	}
	if sameCtx["HAS_CUSTOM_SCRIPT"] != "0" {
		t.Errorf("HAS_CUSTOM_SCRIPT should be '0' when no custom script")
	}
}

func TestInterpolateApp(t *testing.T) {
	app := config.App{
		ID:          "app-1",
		Name:        "my-workload",
		MainContent: "version: '3.8'\nservices:\n  app:\n    image: myrepo/app:${TAG:-latest}\n    environment:\n      - HOST=${NODE_NAME}\n      - ADDR=${NODE_IP}\n",
		EnvContent:  "PORT=${PORT:-8080}\nDOMAIN=${DOMAIN}\n",
		Files: map[string]string{
			"config.json": `{"node":"${NODE_NAME}","region":"${NODE_DATA_REGION:-global}"}`,
		},
	}

	globalVars := map[string]string{
		"DOMAIN": "easy42.net",
		"PORT":   "9000",
	}

	contextVars := map[string]string{
		"NODE_NAME":        "sg-node-01",
		"NODE_IP":          "10.42.0.8",
		"NODE_DATA_REGION": "ap-southeast",
	}

	result := InterpolateApp(app, globalVars, contextVars)

	if !testing.Short() {
		expectedMain := "version: '3.8'\nservices:\n  app:\n    image: myrepo/app:latest\n    environment:\n      - HOST=sg-node-01\n      - ADDR=10.42.0.8\n"
		if result.MainContent != expectedMain {
			t.Errorf("MainContent mismatch:\ngot:\n%s\nwant:\n%s", result.MainContent, expectedMain)
		}

		expectedEnv := "PORT=9000\nDOMAIN=easy42.net\n"
		if result.EnvContent != expectedEnv {
			t.Errorf("EnvContent mismatch:\ngot:\n%s\nwant:\n%s", result.EnvContent, expectedEnv)
		}

		expectedFile := `{"node":"sg-node-01","region":"ap-southeast"}`
		if result.Files["config.json"] != expectedFile {
			t.Errorf("Files[config.json] mismatch:\ngot:\n%s\nwant:\n%s", result.Files["config.json"], expectedFile)
		}
	}
}
