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
// or resolving via upstream provider APIs (like GitHub Releases)
func ResolveArtifactURL(ctx context.Context, spec *config.ArtifactSpec, appName, version, arch, osName string, globalVars map[string]string, client *http.Client) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("artifact spec is nil")
	}

	if osName == "" {
		osName = "linux"
	}
	rawVersion := strings.TrimPrefix(version, "v")
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

		// Lowercase convenience aliases
		"version":     version,
		"raw_version": rawVersion,
		"arch":        arch,
		"arch_gnu":    gnuArch,
		"arch_rust":   rustArch,
		"arch_deb":    debArch,
		"os":          osName,
		"app_name":    appName,
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
			return resolveGitHubRelease(ctx, spec.Resolver, version, arch, osName, globalVars, contextVars, client)
		default:
			return "", fmt.Errorf("unsupported artifact resolver type: %s", spec.Resolver.Type)
		}
	}

	return "", fmt.Errorf("artifact spec has neither url nor resolver configured")
}

func resolveGitHubRelease(ctx context.Context, resolver *config.ArtifactResolver, appVersion, arch, osName string, globalVars, contextVars map[string]string, client *http.Client) (string, error) {
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
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	} else {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, targetVersion)
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
		}, appVersion, arch, osName, globalVars, contextVars, client)
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
	cleanArch := strings.ToLower(arch)
	archVariants := []string{cleanArch}
	switch cleanArch {
	case "amd64":
		archVariants = append(archVariants, "x86_64", "x64")
	case "arm64":
		archVariants = append(archVariants, "aarch64")
	case "armv7":
		archVariants = append(archVariants, "arm", "armhf", "arm-v7", "armv7l")
	case "386":
		archVariants = append(archVariants, "i386", "i686", "x86")
	}
	for _, extra := range []string{ToGNUArch(cleanArch), ToRustArch(cleanArch), ToDebArch(cleanArch)} {
		if extra != "" && extra != cleanArch {
			found := false
			for _, v := range archVariants {
				if v == extra {
					found = true
					break
				}
			}
			if !found {
				archVariants = append(archVariants, extra)
			}
		}
	}

	for _, asset := range rel.Assets {
		name := strings.ToLower(asset.Name)
		if !strings.Contains(name, strings.ToLower(osName)) {
			continue
		}
		for _, a := range archVariants {
			if strings.Contains(name, a) {
				return asset.BrowserDownloadURL, nil
			}
		}
	}

	return "", fmt.Errorf("failed to automatically match release asset for %s %s/%s (found %d assets)", repo, osName, arch, len(rel.Assets))
}
