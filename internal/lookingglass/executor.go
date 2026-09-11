package lookingglass

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"easy42/internal/config"
	easySSH "easy42/internal/ssh"
	"golang.org/x/crypto/ssh"
)

// ExecuteOnNode runs a command on a single node via the SSH ClientPool
func ExecuteOnNode(ctx context.Context, pool *easySSH.ClientPool, node config.Node, command string, parser string, target string, timeout time.Duration) NodeResult {
	start := time.Now()

	result := NodeResult{
		NodeName: node.Name,
		Command:  command,
		Parser:   parser,
	}

	if node.IsExternal {
		result.ExitCode = -1
		result.Error = fmt.Sprintf("Node '%s' is an external unmanaged node; cannot execute commands via SSH", node.Name)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	client, _, err := pool.GetClientWithTimeout(node.Host, 5*time.Second)
	if err != nil {
		result.ExitCode = -1
		result.Error = fmt.Sprintf("SSH connection to '%s' failed: %v", node.Host, err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	session, err := client.NewSession()
	if err != nil {
		result.ExitCode = -1
		result.Error = fmt.Sprintf("Failed to open SSH session: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		result.ExitCode = -1
		result.Error = "Command cancelled by client context"
		result.DurationMs = time.Since(start).Milliseconds()
		return result

	case <-time.After(timeout):
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		result.ExitCode = -1
		result.Error = fmt.Sprintf("Command timed out after %v", timeout)
		result.DurationMs = time.Since(start).Milliseconds()
		result.RawOutput = stdout.String()
		return result

	case runErr := <-done:
		result.DurationMs = time.Since(start).Milliseconds()
		exitCode := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				exitCode = -1
			}
		}
		result.ExitCode = exitCode

		outStr := stdout.String()
		errStr := stderr.String()
		combined := outStr
		if errStr != "" {
			if combined != "" && !strings.HasSuffix(combined, "\n") {
				combined += "\n"
			}
			combined += errStr
		}
		result.RawOutput = combined

		if runErr != nil && combined == "" {
			result.Error = runErr.Error()
		}

		// Parse output
		if parser != "" && parser != "raw" && combined != "" {
			result.Parsed = ParseOutput(parser, combined, target)
		}

		return result
	}
}

// ExecuteMultiNode executes a command across multiple nodes concurrently
func ExecuteMultiNode(ctx context.Context, pool *easySSH.ClientPool, nodes []config.Node, command string, parser string, target string, timeout time.Duration) map[string]NodeResult {
	results := make(map[string]NodeResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, n := range nodes {
		wg.Add(1)
		go func(node config.Node) {
			defer wg.Done()
			res := ExecuteOnNode(ctx, pool, node, command, parser, target, timeout)
			mu.Lock()
			results[node.Name] = res
			mu.Unlock()
		}(n)
	}

	wg.Wait()
	return results
}
