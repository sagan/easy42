package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"

	"easy42/internal/config"
)

// GitHubReleaseAsset represents a GitHub release asset object
type GitHubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// GitHubRelease represents a GitHub release object
type GitHubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []GitHubReleaseAsset `json:"assets"`
}

// ResolveArtifactURL determines the final download URL for an artifact, expanding variables
// or resolving via upstream provider APIs (like GitHub Releases).
//
// The endian parameter ("little"/"big", may be empty) is used to disambiguate MIPS targets:
// upstream projects conventionally name little-endian 32-bit MIPS assets "mipsel" and
// big-endian ones "mips", while the kernel often reports a bare "mips" for both.
func ResolveArtifactURL(ctx context.Context, spec *config.ArtifactSpec, appName, version, arch, osName, endian string, globalVars map[string]string, client *http.Client) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("artifact spec is nil")
	}

	if osName == "" {
		osName = "linux"
	}
	rawVersion := strings.TrimPrefix(version, "v")

	// Settle the MIPS endianness before deriving any of the arch aliases so that
	// ${ARCH}, ${ARCH_GNU}, ${ARCH_RUST} and ${ARCH_DEB} all agree. Rust target
	// triples encode the byte order in the architecture name, so a little-endian
	// MIPS host gets the "el" suffix (mipsel / mips64el) while big-endian does not.
	arch = ResolveMipsEndianness(arch, endian)
	gnuArch := ToGNUArch(arch)
	rustArch := ToRustArch(arch)
	debArch := ToDebArch(arch)

	contextVars := map[string]string{
		"VERSION":     version,
		"RAW_VERSION": rawVersion,
		"ARCH":        arch,
		"ARCH_GNU":    gnuArch,
		"ARCH_RUST":   rustArch,
		"ARCH_DEB":    debArch,
		"OS":          osName,
		"APP_NAME":    appName,
	}

	// Mode A: Direct URL Template
	if spec.URL != "" {
		resolved := InterpolateVariables(spec.URL, globalVars, contextVars)
		return resolved, nil
	}

	// Mode B: GitHub Release Resolver
	if spec.Resolver != nil {
		switch strings.ToLower(spec.Resolver.Type) {
		case "github_release", "github":
			return resolveGitHubRelease(ctx, spec.Resolver, version, arch, osName, endian, globalVars, contextVars, client)
		default:
			return "", fmt.Errorf("unsupported artifact resolver type: %s", spec.Resolver.Type)
		}
	}

	return "", fmt.Errorf("artifact spec has neither url nor resolver configured")
}

// IsMipsArch reports whether arch belongs to the MIPS family (any endianness or bit width).
func IsMipsArch(arch string) bool {
	clean := strings.ToLower(strings.TrimSpace(arch))
	return strings.HasPrefix(clean, "mips")
}

// githubAPIBase is the GitHub REST API root. It is a variable so tests can
// point the resolver at a local server.
var githubAPIBase = "https://api.github.com"

func resolveGitHubRelease(ctx context.Context, resolver *config.ArtifactResolver, appVersion, arch, osName, endian string, globalVars, contextVars map[string]string, client *http.Client) (string, error) {
	repo := strings.TrimSpace(resolver.Repo)
	if repo == "" {
		return "", fmt.Errorf("github resolver repo cannot be empty")
	}

	targetVersion := resolver.Version
	if targetVersion == "" {
		targetVersion = appVersion
	}
	targetVersion = InterpolateVariables(targetVersion, globalVars, contextVars)

	var apiURL string
	if targetVersion == "" || strings.EqualFold(targetVersion, "latest") {
		apiURL = fmt.Sprintf("%s/repos/%s/releases/latest", githubAPIBase, repo)
	} else {
		apiURL = fmt.Sprintf("%s/repos/%s/releases/tags/%s", githubAPIBase, repo, targetVersion)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Easy42-Server/1.0")

	// Support GITHUB_TOKEN from AppVariables or environment
	token := globalVars["GITHUB_TOKEN"]
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound && targetVersion != "" && !strings.HasPrefix(targetVersion, "v") {
		// Fallback: try prepending 'v' if tag without 'v' was 404
		return resolveGitHubRelease(ctx, &config.ArtifactResolver{
			Type:         resolver.Type,
			Repo:         resolver.Repo,
			Version:      "v" + targetVersion,
			AssetPattern: resolver.AssetPattern,
		}, appVersion, arch, osName, endian, globalVars, contextVars, client)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub API returned status %d %s for repo %s", resp.StatusCode, resp.Status, repo)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("failed to parse GitHub release response: %w", err)
	}

	if len(rel.Assets) == 0 {
		return "", fmt.Errorf("no assets found in GitHub release for %s (tag: %s)", repo, rel.TagName)
	}

	// 1. If an asset pattern was provided, use it
	if resolver.AssetPattern != "" {
		pattern := InterpolateVariables(resolver.AssetPattern, globalVars, contextVars)
		for _, asset := range rel.Assets {
			if matched, _ := path.Match(pattern, asset.Name); matched {
				return asset.BrowserDownloadURL, nil
			}
			// Regex fallback if pattern contains regex characters
			if re, err := regexp.Compile(pattern); err == nil && re.MatchString(asset.Name) {
				return asset.BrowserDownloadURL, nil
			}
		}
		return "", fmt.Errorf("no asset matching pattern '%s' found in release %s", pattern, rel.TagName)
	}

	// 2. Automatic asset selection matching arch and osName
	archVariants := archMatchVariants(arch, endian)

	for _, asset := range rel.Assets {
		name := strings.ToLower(asset.Name)
		if !strings.Contains(name, strings.ToLower(osName)) {
			continue
		}
		if assetMatchesArch(name, archVariants) {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("failed to automatically match release asset for %s %s/%s (found %d assets)", repo, osName, arch, len(rel.Assets))
}

// archMatchVariants returns the lowercase architecture spellings to look for in
// upstream asset names, most specific first. For MIPS the endianness reported by
// the agent is used to pick between the "mipsel"/"mips" (or "mips64el"/"mips64")
// spellings, and the opposite-endianness spelling is never returned.
func archMatchVariants(arch, endian string) []string {
	cleanArch := strings.ToLower(strings.TrimSpace(arch))
	variants := []string{cleanArch}
	switch cleanArch {
	case "amd64":
		variants = append(variants, "x86_64", "x64")
	case "arm64":
		variants = append(variants, "aarch64")
	case "armv7":
		variants = append(variants, "arm", "armhf", "arm-v7", "armv7l")
	case "386":
		variants = append(variants, "i386", "i686", "x86")
	}
	for _, extra := range []string{ToGNUArch(cleanArch), ToRustArch(cleanArch), ToDebArch(cleanArch)} {
		if extra != "" && !containsString(variants, extra) {
			variants = append(variants, extra)
		}
	}

	// MIPS assets are named after the byte order, so add the spellings that
	// correspond to the reported endianness only.
	if IsMipsArch(cleanArch) {
		little := NormalizeEndianness(endian) == "little"
		is64 := strings.HasPrefix(cleanArch, "mips64")
		switch {
		case is64 && little:
			variants = append(variants, "mips64el", "mips64le")
		case is64:
			variants = append(variants, "mips64")
		case little:
			variants = append(variants, "mipsel", "mipsle")
		default:
			variants = append(variants, "mips")
		}
	}
	return variants
}

// assetMatchesArch reports whether the lowercase asset name contains one of the
// architecture variants as a whole token. Matching on token boundaries prevents
// "mips" from matching a "mipsel" asset (and vice versa), and "arm" from
// matching "arm64".
func assetMatchesArch(assetName string, variants []string) bool {
	for _, v := range variants {
		if v == "" {
			continue
		}
		idx := 0
		for {
			pos := strings.Index(assetName[idx:], v)
			if pos < 0 {
				break
			}
			start := idx + pos
			end := start + len(v)
			if isArchTokenBoundary(assetName, start-1) && isArchTokenBoundary(assetName, end) {
				return true
			}
			idx = start + 1
		}
	}
	return false
}

// isArchTokenBoundary reports whether the byte at index i separates architecture
// tokens. Indexes outside the string count as boundaries.
func isArchTokenBoundary(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	c := s[i]
	return !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9')
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
