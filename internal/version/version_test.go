package version

import (
	"testing"
)

func TestGetInfo(t *testing.T) {
	info := GetInfo()
	if info.Version == "" {
		t.Errorf("expected non-empty Version")
	}
	if info.AgentVersion != "0.1.0" {
		t.Errorf("expected AgentVersion to be 0.1.0, got %s", info.AgentVersion)
	}
	if info.GitHubURL != "https://github.com/sagan/easy42" {
		t.Errorf("expected GitHubURL to match, got %s", info.GitHubURL)
	}
	if info.GoVersion == "" {
		t.Errorf("expected non-empty GoVersion")
	}
	if info.Platform == "" {
		t.Errorf("expected non-empty Platform")
	}
	if s := info.String(); s == "" {
		t.Errorf("expected non-empty String representation")
	}
}
