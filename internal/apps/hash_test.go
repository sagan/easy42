package apps

import (
	"testing"
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
