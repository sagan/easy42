package config

import (
	"reflect"
	"testing"
)

func TestCleanPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty or nil",
			input:    nil,
			expected: nil,
		},
		{
			name: "broken fragments from split commas in braces",
			input: []string{
				"172.20.0.0/14{21",
				"29}",
				"172.20.0.0/24{28",
				"32}",
				"172.31.0.0/16+",
				"fd00::/8{44",
				"64}",
			},
			expected: []string{
				"172.20.0.0/14{21,29}",
				"172.20.0.0/24{28,32}",
				"172.31.0.0/16+",
				"fd00::/8{44,64}",
			},
		},
		{
			name: "comma-separated prefixes in a single string",
			input: []string{
				"172.20.0.0/14{21, 29}, 172.31.0.0/16+, fd00::/8{44, 64}",
			},
			expected: []string{
				"172.20.0.0/14{21,29}",
				"172.31.0.0/16+",
				"fd00::/8{44,64}",
			},
		},
		{
			name: "newline-separated prefixes in a single string",
			input: []string{
				"172.20.0.0/14{21,29}\n10.0.0.0/8{15,24}\r\nfd00::/8{44,64}",
			},
			expected: []string{
				"172.20.0.0/14{21,29}",
				"10.0.0.0/8{15,24}",
				"fd00::/8{44,64}",
			},
		},
		{
			name: "already clean prefixes",
			input: []string{
				"172.20.0.0/14{21,29}",
				"fd00::/8{44,64}",
			},
			expected: []string{
				"172.20.0.0/14{21,29}",
				"fd00::/8{44,64}",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanPrefixes(tc.input)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Fatalf("CleanPrefixes mismatch:\ngot:      %v\nexpected: %v", got, tc.expected)
			}
		})
	}
}
