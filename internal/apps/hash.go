package apps

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"strings"

	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
)

// ComputeAppHash returns a deterministic SHA-256 hash of an App definition
func ComputeAppHash(app config.App) string {
	h := sha256.New()
	fmt.Fprintf(h, "name:%s\n", app.Name)
	fmt.Fprintf(h, "type:%s\n", app.AppType)
	fmt.Fprintf(h, "enabled:%t\n", app.Enabled)
	fmt.Fprintf(h, "main:%s\n", app.MainContent)
	fmt.Fprintf(h, "env:%s\n", app.EnvContent)

	if len(app.Files) > 0 {
		var keys []string
		for k := range app.Files {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(h, "file:%s:%s\n", k, app.Files[k])
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// ComputeAppFilesHash returns a deterministic SHA-256 hash of deployed app service & env files,
// matching the algorithm used on the agent.
func ComputeAppFilesHash(mainContent, envContent string, files map[string]string) string {
	h := sha256.New()
	normMain := strings.TrimRight(strings.ReplaceAll(mainContent, "\r\n", "\n"), " \t\r\n")
	normEnv := strings.TrimRight(strings.ReplaceAll(envContent, "\r\n", "\n"), " \t\r\n")

	fmt.Fprintf(h, "main:\n%s\n", normMain)
	fmt.Fprintf(h, "env:\n%s\n", normEnv)

	if len(files) > 0 {
		keys := make([]string, 0, len(files))
		for k := range files {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			normFile := strings.TrimRight(strings.ReplaceAll(files[k], "\r\n", "\n"), " \t\r\n")
			fmt.Fprintf(h, "file:%s:\n%s\n", k, normFile)
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// ComputeAppSpecFilesHash computes the files hash for a proto AppSpec
func ComputeAppSpecFilesHash(spec *agentpb.AppSpec) string {
	if spec == nil {
		return ""
	}
	files := make(map[string]string, len(spec.ConfigFiles))
	for _, f := range spec.ConfigFiles {
		if f != nil {
			files[f.Path] = string(f.Content)
		}
	}
	return ComputeAppFilesHash(spec.MainContent, spec.EnvContent, files)
}

// ComputeAppArtifactHash returns a deterministic SHA-256 fingerprint of the
// artifact files that a node is expected to refresh. Artifacts using the
// "if_not_present" or "never" pull policies are ignored because a server-side
// cache update does not change what those nodes would deploy.
func ComputeAppArtifactHash(spec *agentpb.AppSpec) string {
	if spec == nil || len(spec.Artifacts) == 0 {
		return ""
	}

	entries := make([]string, 0, len(spec.Artifacts))
	for _, artifact := range spec.Artifacts {
		if artifact == nil {
			continue
		}
		policy := strings.ToLower(strings.TrimSpace(artifact.PullPolicy))
		if policy == "" {
			policy = "if_changed"
		}
		if policy != "if_changed" && policy != "always" {
			continue
		}
		entries = append(entries, fmt.Sprintf("%s\x00%s\x00%s\x00%d",
			artifact.Name, artifact.TargetPath, artifact.FileSha256, artifact.FileMode))
	}
	if len(entries) == 0 {
		return ""
	}

	sort.Strings(entries)
	h := sha256.New()
	for _, entry := range entries {
		fmt.Fprintf(h, "artifact:%s\n", entry)
	}
	return hex.EncodeToString(h.Sum(nil))
}
