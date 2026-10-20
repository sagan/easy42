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

// TaskSourceFile represents a source script or configuration file belonging to a task or shared across tasks
type TaskSourceFile struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Shared  bool   `json:"shared"`
}

// GetTaskSources returns all script/source files for a specific task, plus shared files like common.sh
func GetTaskSources(taskID string) ([]TaskSourceFile, error) {
	if _, err := GetTaskMeta(taskID); err != nil {
		return nil, err
	}

	var files []TaskSourceFile

	entries, err := fs.ReadDir(tasksEmbed, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to read task dir %s: %w", taskID, err)
	}

	var taskFiles []TaskSourceFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(taskID, entry.Name())
		content, err := tasksEmbed.ReadFile(filePath)
		if err != nil {
			continue
		}
		taskFiles = append(taskFiles, TaskSourceFile{
			Name:    entry.Name(),
			Path:    filePath,
			Content: string(content),
			Shared:  false,
		})
	}

	filePriority := func(name string) int {
		switch name {
		case "run.sh":
			return 1
		case "status.sh":
			return 2
		case "source.sh":
			return 3
		default:
			if strings.HasSuffix(name, ".sh") {
				return 4
			}
			return 5
		}
	}
	sort.Slice(taskFiles, func(i, j int) bool {
		pI, pJ := filePriority(taskFiles[i].Name), filePriority(taskFiles[j].Name)
		if pI != pJ {
			return pI < pJ
		}
		return taskFiles[i].Name < taskFiles[j].Name
	})

	files = append(files, taskFiles...)

	commonEntries, err := fs.ReadDir(tasksEmbed, "_common")
	if err == nil {
		var commonFiles []TaskSourceFile
		for _, entry := range commonEntries {
			if entry.IsDir() {
				continue
			}
			commonPath := filepath.Join("_common", entry.Name())
			content, err := tasksEmbed.ReadFile(commonPath)
			if err != nil {
				continue
			}
			commonFiles = append(commonFiles, TaskSourceFile{
				Name:    entry.Name(),
				Path:    commonPath,
				Content: string(content),
				Shared:  true,
			})
		}
		sort.Slice(commonFiles, func(i, j int) bool {
			return commonFiles[i].Name < commonFiles[j].Name
		})
		files = append(files, commonFiles...)
	}

	return files, nil
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
	extraFiles ...map[string][]byte,
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

	// Write extra payload files if provided
	if len(extraFiles) > 0 && extraFiles[0] != nil {
		for fname, content := range extraFiles[0] {
			extraPath := filepath.Join(remoteDir, fname)
			if ef, err := sftpClient.OpenFile(extraPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC); err == nil {
				_, _ = ef.Write(content)
				_ = ef.Close()
				perm := os.FileMode(0644)
				switch fname {
				case "easy42-agent", "status.sh", "run.sh":
					perm = 0755
				case "agent.json":
					perm = 0600
				}
				_ = sftpClient.Chmod(extraPath, perm)
			}
		}
	}

	// Execute remote script
	execCmd := fmt.Sprintf("sh %s", remoteScriptPath)
	stdout, stderr, exitCode, runErr := ssh.RunCommandWithExitCode(sshClient, execCmd)
	durationMs = time.Since(start).Milliseconds()

	return stdout, stderr, exitCode, durationMs, runErr
}

// ReadTaskScript reads an embedded task script
func ReadTaskScript(taskID string, scriptName string) ([]byte, error) {
	return tasksEmbed.ReadFile(filepath.Join(taskID, scriptName))
}

// CheckStatus checks if a task is executable or already done on a remote host
func CheckStatus(
	sshClient *golangssh.Client,
	sftpClient *sftp.Client,
	taskID string,
	nodeName string,
) TaskStatusResult {
	return CheckStatusWithPayload(sshClient, sftpClient, taskID, nodeName, nil)
}

// CheckStatusWithPayload checks if a task is executable or already done on a remote host with extra payload files uploaded
func CheckStatusWithPayload(
	sshClient *golangssh.Client,
	sftpClient *sftp.Client,
	taskID string,
	nodeName string,
	payload map[string][]byte,
) TaskStatusResult {
	res := TaskStatusResult{
		TaskID:   taskID,
		NodeName: nodeName,
	}

	stdout, stderr, exitCode, duration, err := executeScript(sshClient, sftpClient, taskID, "status.sh", payload)
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

// RunTaskWithPayload executes the task run.sh on a remote host with extra payload files uploaded
func RunTaskWithPayload(
	sshClient *golangssh.Client,
	sftpClient *sftp.Client,
	taskID string,
	nodeName string,
	payload map[string][]byte,
) TaskRunResult {
	res := TaskRunResult{
		TaskID:   taskID,
		NodeName: nodeName,
	}

	stdout, stderr, exitCode, duration, err := executeScript(sshClient, sftpClient, taskID, "run.sh", payload)
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
