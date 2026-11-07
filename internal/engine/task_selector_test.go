package engine

import (
	"testing"

	"easy42/internal/config"
)

func TestNodeMatchesTaskSelector(t *testing.T) {
	node := config.Node{
		Name:                "edge-1",
		Interface:           "eth0",
		IP:                  "192.168.100.1/24",
		IP6:                 "fd42:a159:f9f0::1/64",
		ExternalIP:          "172.20.1.1",
		Tags:                []string{"edge", "prod"},
		CustomNetworkScript: "#!/bin/sh\n",
		Note:                "hello\n```json\n{\"role\":\"gateway\",\"meta\":{\"region\":\"eu\"}}\n```\n",
	}

	tests := []struct {
		name     string
		selector map[string]string
		want     bool
	}{
		{name: "nil selector", selector: nil, want: true},
		{name: "empty selector", selector: map[string]string{}, want: true},
		{name: "matching name", selector: map[string]string{"name": "edge-1,edge-2"}, want: true},
		{name: "non-matching name", selector: map[string]string{"name": "edge-2,edge-3"}, want: false},
		{name: "empty name list", selector: map[string]string{"name": " , "}, want: false},
		{name: "matching tag", selector: map[string]string{"tag": "core, prod"}, want: true},
		{name: "non-matching tag", selector: map[string]string{"tag": "core,cloud"}, want: false},
		{name: "matching ip", selector: map[string]string{"env:NODE_IP": "192.168.100.1"}, want: true},
		{name: "non-matching ip", selector: map[string]string{"env:NODE_IP": "192.168.100.2"}, want: false},
		{name: "matching ipv6", selector: map[string]string{"env:NODE_IP6": "fd42:a159:f9f0::1"}, want: true},
		{name: "matching note data", selector: map[string]string{"env:NODE_DATA_ROLE": "gateway"}, want: true},
		{name: "matching nested note data", selector: map[string]string{"env:NODE_DATA_META_REGION": "eu"}, want: true},
		{name: "matching note data alias", selector: map[string]string{"env:NODE_DATA_REGION": "eu"}, want: true},
		{name: "unknown env", selector: map[string]string{"env:NODE_DOES_NOT_EXIST": ""}, want: false},
		{name: "lowercase env key", selector: map[string]string{"env:node_ip": "192.168.100.1"}, want: true},
		{
			name:     "all conditions match (AND)",
			selector: map[string]string{"name": "edge-1", "tag": "edge", "env:NODE_DATA_ROLE": "gateway"},
			want:     true,
		},
		{
			name:     "one condition fails (AND)",
			selector: map[string]string{"name": "edge-1", "tag": "core"},
			want:     false,
		},
		{name: "unknown selector key fails closed", selector: map[string]string{"bogus": "value"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NodeMatchesTaskSelector(node, tt.selector); got != tt.want {
				t.Fatalf("NodeMatchesTaskSelector() = %v, want %v", got, tt.want)
			}
		})
	}
}
