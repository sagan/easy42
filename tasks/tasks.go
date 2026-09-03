package tasks

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"easy42/internal/ssh"
	"github.com/pkg/sftp"
	golangssh "golang.org/x/crypto/ssh"
)

//go:embed all:*
var tasksEmbed embed.FS

// TaskMeta represents metadata of an available helper task
type TaskMeta struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Weight      int    `json:"weight"`
}

// TaskStatusResult represents the result of checking status on a node
type TaskStatusResult struct {
	TaskID     string `json:"task_id"`
	NodeName   string `json:"node_name"`
	Status     string `json:"status"` // "ready", "done", "incompatible", "error"
	Message    string `json:"message"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

// TaskRunResult represents the result of executing a task on a node
type TaskRunResult struct {
	TaskID     string `json:"task_id"`
	NodeName   string `json:"node_name"`
	Success    bool   `json:"success"`
	Output     string `json:"output"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

// GetTasks returns all available tasks sorted by weight
func GetTasks() ([]TaskMeta, error) {
	entries, err := fs.ReadDir(tasksEmbed, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded tasks root: %w", err)
	}

	var tasks []TaskMeta
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}

		manifestPath := filepath.Join(entry.Name(), "task.json")
		data, err := tasksEmbed.ReadFile(manifestPath)
		if err != nil {
			// Skip directories without task.json
			continue
		}

		var meta TaskMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		if meta.ID == "" {
			meta.ID = entry.Name()
		}
		tasks = append(tasks, meta)
	}

	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Weight != tasks[j].Weight {
			return tasks[i].Weight < tasks[j].Weight
		}
		return tasks[i].ID < tasks[j].ID
	})

	return tasks, nil
}

// GetTaskMeta returns metadata for a single task
func GetTaskMeta(taskID string) (*TaskMeta, error) {
	tasks, err := GetTasks()
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if t.ID == taskID {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("task '%s' not found", taskID)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// executeScript uploads common.sh and the target script to a remote temp dir and executes it
func executeScript(
	sshClient *golangssh.Client,
	sftpClient *sftp.Client,
	taskID string,
	scriptName string,
) (stdout string, stderr string, exitCode int, durationMs int64, err error) {
	start := time.Now()

	// Read common.sh if available
	commonScript, _ := tasksEmbed.ReadFile("_common/common.sh")

	// Read target script
	scriptPath := filepath.Join(taskID, scriptName)
	scriptContent, err := tasksEmbed.ReadFile(scriptPath)
	if err != nil {
		return "", "", -1, 0, fmt.Errorf("failed to read task script %s: %w", scriptPath, err)
	}

	// Create a unique remote temporary folder in /tmp
	randID := randomHex(6)
	remoteDir := fmt.Sprintf("/tmp/.easy42_task_%s_%s", taskID, randID)
	if err := sftpClient.MkdirAll(remoteDir); err != nil {
		return "", "", -1, 0, fmt.Errorf("failed to create remote tmp dir %s: %w", remoteDir, err)
	}

	// Ensure cleanup on return
	defer func() {
		_, _ = ssh.RunCommand(sshClient, fmt.Sprintf("rm -rf %s", remoteDir))
	}()

	// Write common.sh if available
	if len(commonScript) > 0 {
		commonPath := filepath.Join(remoteDir, "common.sh")
		if f, err := sftpClient.OpenFile(commonPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC); err == nil {
			_, _ = f.Write(commonScript)
			_ = f.Close()
			_ = sftpClient.Chmod(commonPath, 0755)
		}
	}

	// Write target script
	remoteScriptPath := filepath.Join(remoteDir, scriptName)
	f, err := sftpClient.OpenFile(remoteScriptPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return "", "", -1, 0, fmt.Errorf("failed to write remote script %s: %w", remoteScriptPath, err)
	}
	if _, err := f.Write(scriptContent); err != nil {
		_ = f.Close()
		return "", "", -1, 0, fmt.Errorf("failed to write content to %s: %w", remoteScriptPath, err)
	}
	_ = f.Close()
	_ = sftpClient.Chmod(remoteScriptPath, 0755)

	// Execute remote script
	execCmd := fmt.Sprintf("sh %s", remoteScriptPath)
	stdout, stderr, exitCode, runErr := ssh.RunCommandWithExitCode(sshClient, execCmd)
	durationMs = time.Since(start).Milliseconds()

	return stdout, stderr, exitCode, durationMs, runErr
}

// CheckStatus checks if a task is executable or already done on a remote host
func CheckStatus(
	sshClient *golangssh.Client,
	sftpClient *sftp.Client,
	taskID string,
	nodeName string,
) TaskStatusResult {
	res := TaskStatusResult{
		TaskID:   taskID,
		NodeName: nodeName,
	}

	stdout, stderr, exitCode, duration, err := executeScript(sshClient, sftpClient, taskID, "status.sh")
	res.DurationMs = duration
	res.ExitCode = exitCode

	msg := strings.TrimSpace(stdout)
	if msg == "" {
		msg = strings.TrimSpace(stderr)
	}
	if msg == "" && err != nil {
		msg = err.Error()
	}
	res.Message = msg

	switch exitCode {
	case 0:
		res.Status = "ready"
	case 10:
		res.Status = "done"
	case 20:
		res.Status = "incompatible"
	default:
		res.Status = "error"
		if res.Message == "" {
			res.Message = fmt.Sprintf("Command failed with exit code %d", exitCode)
		}
	}

	return res
}

// RunTask executes the task run.sh on a remote host
func RunTask(
	sshClient *golangssh.Client,
	sftpClient *sftp.Client,
	taskID string,
	nodeName string,
) TaskRunResult {
	res := TaskRunResult{
		TaskID:   taskID,
		NodeName: nodeName,
	}

	stdout, stderr, exitCode, duration, err := executeScript(sshClient, sftpClient, taskID, "run.sh")
	res.DurationMs = duration
	res.ExitCode = exitCode

	output := strings.TrimSpace(stdout)
	if stderrOutput := strings.TrimSpace(stderr); stderrOutput != "" {
		if output != "" {
			output += "\n" + stderrOutput
		} else {
			output = stderrOutput
		}
	}
	if output == "" && err != nil {
		output = err.Error()
	}
	res.Output = output

	if exitCode == 0 && err == nil {
		res.Success = true
	} else {
		res.Success = false
	}

	return res
}
