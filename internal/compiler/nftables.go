package compiler

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"easy42/internal/config"
	"easy42/util/tplutil"
)

// DefaultNftablesConfigPath is the standard destination path for generated nftables rules on managed devices
const DefaultNftablesConfigPath = "/etc/easy42.nft"

// GetDefaultNftablesTemplate returns the content of the embedded easy42.nft.tmpl template
func GetDefaultNftablesTemplate() (string, error) {
	data, err := templatesFS.ReadFile("templates/easy42.nft.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to read embedded nftables template: %w", err)
	}
	return string(data), nil
}

// FormatNftPrefixList formats a list of IP prefixes into an nftables set elements string like "{ 172.20.0.0/14 }"
// It strips BIRD-specific syntax (e.g. {21,29} or +), deduplicates, and filters out overlapping/redundant subnets
// to prevent nftables from failing with "Error: conflicting intervals specified".
func FormatNftPrefixList(prefixes []string, defaultList []string, isV6 bool) string {
	cleaned := config.CleanPrefixes(prefixes)
	var parsedList []netip.Prefix

	for _, raw := range cleaned {
		pStr := strings.TrimSpace(raw)
		if pStr == "" {
			continue
		}
		// Strip BIRD prefix length constraint braces e.g. {21,29}
		if idx := strings.Index(pStr, "{"); idx != -1 {
			pStr = strings.TrimSpace(pStr[:idx])
		}
		// Strip BIRD trailing '+' or '-'
		pStr = strings.TrimRight(pStr, "+-")

		// If no mask specified, append /32 for v4 or /128 for v6
		if !strings.Contains(pStr, "/") {
			if strings.Contains(pStr, ":") {
				pStr += "/128"
			} else {
				pStr += "/32"
			}
		}

		p, err := netip.ParsePrefix(pStr)
		if err != nil {
			continue
		}

		if isV6 && p.Addr().Is6() {
			parsedList = append(parsedList, p.Masked())
		} else if !isV6 && p.Addr().Is4() {
			parsedList = append(parsedList, p.Masked())
		}
	}

	// Filter out nested subnets to avoid nftables conflicting interval error
	// If prefix b contains prefix a (where b has fewer bits / larger subnet), omit a
	var nonOverlapping []netip.Prefix
	for i, a := range parsedList {
		isCovered := false
		for j, b := range parsedList {
			if i != j {
				// If b has shorter prefix and b contains a
				if b.Bits() < a.Bits() && b.Contains(a.Addr()) {
					isCovered = true
					break
				}
				// If same bits and same prefix, deduplicate by index
				if b.Bits() == a.Bits() && b == a && j < i {
					isCovered = true
					break
				}
			}
		}
		if !isCovered {
			nonOverlapping = append(nonOverlapping, a)
		}
	}

	// Sort prefixes for stable output
	sort.Slice(nonOverlapping, func(i, j int) bool {
		return nonOverlapping[i].String() < nonOverlapping[j].String()
	})

	var result []string
	for _, p := range nonOverlapping {
		result = append(result, p.String())
	}

	if len(result) == 0 {
		result = defaultList
	}

	return "{ " + strings.Join(result, ", ") + " }"
}

// BuildNftablesNodeContext prepares the template context specifically for nftables generation
func BuildNftablesNodeContext(
	node *config.Node,
	allNodes []config.Node,
	links []config.Link,
	netSettings ...*config.NetworkSettings,
) (map[string]any, error) {
	ctx, err := BuildNodeContext(node, allNodes, links, netSettings...)
	if err != nil {
		return nil, err
	}

	var prefixes []string
	if len(netSettings) > 0 && netSettings[0] != nil {
		prefixes = netSettings[0].Prefixes
	}

	ctx["nft_prefixes_v4"] = FormatNftPrefixList(prefixes, []string{"172.20.0.0/14"}, false)
	ctx["nft_prefixes_v6"] = FormatNftPrefixList(prefixes, []string{"fd00::/8"}, true)

	return ctx, nil
}

// GenerateNftablesConfigWithTemplate generates nftables config using a custom template
func GenerateNftablesConfigWithTemplate(
	tmplContent string,
	node *config.Node,
	allNodes []config.Node,
	links []config.Link,
	netSettings ...*config.NetworkSettings,
) (string, error) {
	ctx, err := BuildNftablesNodeContext(node, allNodes, links, netSettings...)
	if err != nil {
		return "", err
	}

	return tplutil.RenderTemplate(tmplContent, ctx)
}

// GenerateNftablesConfig compiles the nftables configuration for a node using the default embedded template
func GenerateNftablesConfig(
	node *config.Node,
	allNodes []config.Node,
	links []config.Link,
	netSettings ...*config.NetworkSettings,
) (string, error) {
	tmplContent, err := GetDefaultNftablesTemplate()
	if err != nil {
		return "", err
	}
	return GenerateNftablesConfigWithTemplate(tmplContent, node, allNodes, links, netSettings...)
}
