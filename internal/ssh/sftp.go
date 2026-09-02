package ssh

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
)

// ReadRemoteFile reads the entire content of a file via SFTP
func ReadRemoteFile(client *sftp.Client, path string) (string, error) {
	f, err := client.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// AtomicWriteFile writes content to a temporary file in the target directory and renames it
func AtomicWriteFile(client *sftp.Client, targetPath string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)

	// Ensure directory exists
	if err := client.MkdirAll(dir); err != nil {
		// Ignore error if directory already exists
	}

	tmpPath := filepath.Join(dir, fmt.Sprintf(".%s.tmp", base))

	// Remove old tmp file if exists
	_ = client.Remove(tmpPath)

	tmpFile, err := client.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("failed to open temp file %s: %w", tmpPath, err)
	}

	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		_ = client.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file %s: %w", tmpPath, err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = client.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file %s: %w", tmpPath, err)
	}

	if err := client.Chmod(tmpPath, perm); err != nil {
		// Non-fatal, continue
	}

	// In SFTP, Remove target before Rename if OS doesn't support atomic overwrite
	_ = client.Remove(targetPath)
	if err := client.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tmpPath, targetPath, err)
	}

	return nil
}
