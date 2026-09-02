package ssh

import (
	"reflect"
	"testing"
)

func TestParseWgInterfaces(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []string
	}{
		{
			name:     "empty output",
			output:   "",
			expected: nil,
		},
		{
			name: "standard wg output single interface",
			output: `interface: wg42tokyo
  public key: qI1moNwNfwUP6TtikaBIARfrT1vfjbSRX7tKFNOkkng=
  private key: (hidden)
  listening port: 25882

peer: aGSsGvA14ToH3whg+hi72JxBF5C1UOnrld5g2EY/nVo=
  endpoint: 172.24.3.244:24309
  allowed ips: fe80::ac18:3f4/128, 0.0.0.0/0, ::/0
  latest handshake: 53 seconds ago
`,
			expected: []string{"wg42tokyo"},
		},
		{
			name: "multiple interfaces including non-wg42",
			output: `interface: wg42london
  public key: key1
  listening port: 51820

interface: wg0
  public key: key2
  listening port: 51821

interface: wg42paris
  public key: key3
  listening port: 51822
`,
			expected: []string{"wg42london", "wg0", "wg42paris"},
		},
		{
			name:     "space separated interfaces",
			output:   "wg42node1 wg42node2 eth0\n",
			expected: []string{"wg42node1", "wg42node2", "eth0"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := ParseWgInterfaces(tc.output)
			if len(actual) == 0 && len(tc.expected) == 0 {
				return
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}
