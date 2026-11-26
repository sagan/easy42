package apps

import (
	"strings"
	"testing"
)

func TestInjectScriptEnvironmentShell(t *testing.T) {
	main := `#!/bin/sh
echo "starting script"
echo "IP is $IP"
`
	env := "IP=192.168.100.1\nPORT=8080"
	got := InjectScriptEnvironment(main, env)

	expectedBlock := `### EASY42_ENV_START
export IP='192.168.100.1'
export PORT='8080'
### EASY42_ENV_END`

	if !strings.Contains(got, expectedBlock) {
		t.Fatalf("expected output to contain block:\n%s\ngot:\n%s", expectedBlock, got)
	}

	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) < 2 || lines[0] != "#!/bin/sh" {
		t.Fatalf("first line must be #!/bin/sh, got: %s", lines[0])
	}
}

func TestInjectScriptEnvironmentPython(t *testing.T) {
	main := `#!/usr/bin/env python3
import sys

print("Hello Python")
`
	env := "IP=192.168.100.1\nMSG='hello world'"
	got := InjectScriptEnvironment(main, env)

	expectedBlock := `### EASY42_ENV_START
import os
os.environ["IP"] = "192.168.100.1"
os.environ["MSG"] = "hello world"
### EASY42_ENV_END`

	if !strings.Contains(got, expectedBlock) {
		t.Fatalf("expected python block:\n%s\ngot:\n%s", expectedBlock, got)
	}

	if !strings.HasPrefix(got, "#!/usr/bin/env python3\n\n### EASY42_ENV_START") {
		t.Fatalf("shebang should be followed by env block, got:\n%s", got)
	}
}

func TestInjectScriptEnvironmentNoShebang(t *testing.T) {
	main := `echo "no shebang here"`
	env := "IP=10.0.0.1"
	got := InjectScriptEnvironment(main, env)

	if !strings.HasPrefix(got, "#!/bin/sh\n\n### EASY42_ENV_START\nexport IP='10.0.0.1'\n### EASY42_ENV_END") {
		t.Fatalf("expected auto-shebang and env block, got:\n%s", got)
	}
}

func TestInjectScriptEnvironmentReplacesExistingBlock(t *testing.T) {
	main := `#!/bin/sh

### EASY42_ENV_START
export OLD_IP=1.1.1.1
### EASY42_ENV_END

echo "run"
`
	env := "IP=2.2.2.2"
	got := InjectScriptEnvironment(main, env)

	if strings.Contains(got, "OLD_IP") {
		t.Fatalf("old env block was not removed:\n%s", got)
	}
	if !strings.Contains(got, "export IP='2.2.2.2'") {
		t.Fatalf("new env var not found:\n%s", got)
	}
	// Verify only one START and END marker
	if strings.Count(got, "### EASY42_ENV_START") != 1 || strings.Count(got, "### EASY42_ENV_END") != 1 {
		t.Fatalf("expected exactly one env block marker, got:\n%s", got)
	}
}

func TestInjectScriptEnvironmentEmptyEnv(t *testing.T) {
	main := `#!/bin/sh
echo "clean"
`
	got := InjectScriptEnvironment(main, "")
	if strings.Contains(got, "EASY42_ENV_START") {
		t.Fatalf("expected no env block for empty env, got:\n%s", got)
	}
	if !strings.Contains(got, "echo \"clean\"") {
		t.Fatalf("expected main content to be preserved, got:\n%s", got)
	}
}
