package apps

import (
	"testing"

	agentpb "easy42/internal/agent/proto"
)

func TestComputeAppFilesHash(t *testing.T) {
	main := "[Unit]\nDescription=rclone\n"
	env := "RCLONE_PASS=123456\r\n"
	files := map[string]string{
		"conf/rclone.conf": "[drive]\ntype=drive\n",
	}

	h1 := ComputeAppFilesHash(main, env, files)
	if len(h1) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(h1))
	}

	// CRLF & trailing whitespace normalization
	main2 := "[Unit]\nDescription=rclone   \r\n"
	env2 := "RCLONE_PASS=123456\n\n"
	h2 := ComputeAppFilesHash(main2, env2, files)
	if h1 != h2 {
		t.Fatalf("expected hashes to match after normalization: %s != %s", h1, h2)
	}

	// Change in content should change hash
	main3 := "[Unit]\nDescription=rclone modified\n"
	h3 := ComputeAppFilesHash(main3, env, files)
	if h1 == h3 {
		t.Fatalf("expected different hash when content modified")
	}
}

func TestComputeAppArtifactHash(t *testing.T) {
	if got := ComputeAppArtifactHash(nil); got != "" {
		t.Fatalf("expected empty hash for nil spec, got %s", got)
	}
	if got := ComputeAppArtifactHash(&agentpb.AppSpec{}); got != "" {
		t.Fatalf("expected empty hash for spec without artifacts, got %s", got)
	}

	spec := &agentpb.AppSpec{
		Artifacts: []*agentpb.AppArtifactFile{
			{Name: "rclone", TargetPath: "/usr/local/bin/rclone", FileSha256: "aaa", FileMode: 0755, PullPolicy: "if_changed"},
		},
	}
	h1 := ComputeAppArtifactHash(spec)
	if len(h1) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(h1))
	}

	// Default policy is treated as if_changed, so an empty policy must not
	// change the fingerprint.
	spec.Artifacts[0].PullPolicy = ""
	if h2 := ComputeAppArtifactHash(spec); h1 != h2 {
		t.Fatalf("expected empty pull policy to match if_changed: %s != %s", h1, h2)
	}

	// Artifact content change must change the fingerprint, even if the app
	// version label remains "latest".
	spec.Artifacts[0].PullPolicy = "if_changed"
	spec.Artifacts[0].FileSha256 = "bbb"
	h3 := ComputeAppArtifactHash(spec)
	if h1 == h3 {
		t.Fatalf("expected different hash when artifact sha256 changes")
	}

	// Pull policies that never refresh an existing artifact must not create
	// drift when the server cache is updated.
	for _, policy := range []string{"if_not_present", "never"} {
		spec.Artifacts[0].PullPolicy = policy
		if got := ComputeAppArtifactHash(spec); got != "" {
			t.Fatalf("expected empty hash for pull policy %q, got %s", policy, got)
		}
	}
}
