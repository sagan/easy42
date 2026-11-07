package engine

import "easy42/internal/config"

// NodeMatchesTaskSelector reports whether a node satisfies every condition in a
// task selector. An empty selector matches every node.
//
// Supported selector keys:
//   - name: comma-separated whitelist of node names.
//   - tag: comma-separated whitelist of node tags; the node must have at least
//     one of them.
//   - env:<NAME>: the node metadata/environment variable must equal the value.
//     This includes built-ins such as NODE_IP/NODE_IP6 and NODE_DATA_* values
//     extracted from the node Note JSON block.
func NodeMatchesTaskSelector(n config.Node, selector map[string]string) bool {
	return config.NodeMatchesSelector(n, selector)
}
