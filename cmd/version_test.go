package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetArgs([]string{"version"})

	err := RootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error executing version command: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "easy42") {
		t.Errorf("expected output to contain 'easy42', got: %s", output)
	}
}
