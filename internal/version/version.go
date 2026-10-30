package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

var (
	// Version is the current version of easy42, injected at build time via -ldflags
	Version = "dev"
	// Commit is the git commit SHA, injected at build time via -ldflags
	Commit = "none"
	// Date is the build date ISO timestamp, injected at build time via -ldflags
	Date = "unknown"
	// BuiltBy is the builder tool or persona, injected at build time via -ldflags
	BuiltBy = "unknown"
)

const (
	// AgentVersion is the version of the bundled easy42-agent
	AgentVersion = "0.1.0"
	// GitHubURL is the project repository URL
	GitHubURL = "https://github.com/sagan/easy42"
)

// Info encapsulates comprehensive build and version details.
type Info struct {
	Version       string `json:"version"`
	Commit        string `json:"commit,omitempty"`
	Date          string `json:"date,omitempty"`
	BuiltBy       string `json:"built_by,omitempty"`
	GoVersion     string `json:"go_version"`
	Platform      string `json:"platform"`
	AgentVersion  string `json:"agent_version"`
	GitHubURL     string `json:"github_url"`
	ListenAddress string `json:"listen_address,omitempty"`
	DataDir       string `json:"data_dir,omitempty"`
	ConfigFile    string `json:"config_file,omitempty"`
}

func init() {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			Version = bi.Main.Version
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if Commit == "none" {
					Commit = s.Value
				}
			case "vcs.time":
				if Date == "unknown" {
					Date = s.Value
				}
			}
		}
	}
}

// GetInfo returns complete version information for easy42.
func GetInfo() Info {
	return Info{
		Version:      Version,
		Commit:       Commit,
		Date:         Date,
		BuiltBy:      BuiltBy,
		GoVersion:    runtime.Version(),
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		AgentVersion: AgentVersion,
		GitHubURL:    GitHubURL,
	}
}

// String returns a formatted version string.
func (i Info) String() string {
	if i.Commit != "" && i.Commit != "none" {
		return fmt.Sprintf("%s (%s)", i.Version, i.Commit)
	}
	return i.Version
}
