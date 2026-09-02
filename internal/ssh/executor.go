package ssh

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"easy42/internal/config"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// RunCommand executes a command on the remote host and returns stdout/stderr
func RunCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create ssh session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(command); err != nil {
		errOutput := strings.TrimSpace(stderr.String())
		if errOutput == "" {
			errOutput = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), fmt.Errorf("command failed (%s): %w - %s", command, err, errOutput)
	}

	return stdout.String(), nil
}

// IsInterfaceUp checks if a network interface is present and UP
func IsInterfaceUp(client *ssh.Client, iface string) bool {
	out, err := RunCommand(client, fmt.Sprintf("ip link show %s", iface))
	if err != nil {
		return false
	}
	return strings.Contains(out, "state UP") || strings.Contains(out, "state UNKNOWN")
}

// InterfaceExists checks if a network interface exists
func InterfaceExists(client *ssh.Client, iface string) bool {
	_, err := RunCommand(client, fmt.Sprintf("ip link show %s", iface))
	return err == nil
}

// UpWireGuard starts an interface using wg-quick up
func UpWireGuard(client *ssh.Client, iface string) error {
	_, err := RunCommand(client, fmt.Sprintf("wg-quick up %s", iface))
	return err
}

// DownWireGuard stops an interface using wg-quick down
func DownWireGuard(client *ssh.Client, iface string) error {
	_, err := RunCommand(client, fmt.Sprintf("wg-quick down %s", iface))
	return err
}

// IsInterfaceStarted checks if a network interface is in started state (exists, is up, and recognized by wireguard)
func IsInterfaceStarted(client *ssh.Client, iface string) bool {
	out, err := RunCommand(client, fmt.Sprintf("ip link show %s", iface))
	if err != nil {
		return false
	}
	// An interface is UP if flags contain UP and state is not DOWN
	isUp := (strings.Contains(out, "state UP") || strings.Contains(out, "state UNKNOWN") || strings.Contains(out, ",UP")) && !strings.Contains(out, "state DOWN")
	if !isUp {
		return false
	}
	// Also ensure wireguard recognizes it
	_, wgErr := RunCommand(client, fmt.Sprintf("wg show %s", iface))
	return wgErr == nil
}

// StartWireGuardInterface brings up a WireGuard interface using wg-quick up.
// If a stale or unconfigured interface device with the same name exists, it tears it down first to avoid "already exists" errors.
func StartWireGuardInterface(client *ssh.Client, iface string) error {
	if InterfaceExists(client, iface) {
		_ = DownWireGuard(client, iface)
		_, _ = RunCommand(client, fmt.Sprintf("ip link del dev %s", iface))
	}
	return UpWireGuard(client, iface)
}

// ParseWgInterfaces parses output of "wg" command (or "wg show interfaces")
func ParseWgInterfaces(output string) []string {
	var ifaces []string
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "interface:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := strings.TrimSpace(parts[1])
				if name != "" && !seen[name] {
					seen[name] = true
					ifaces = append(ifaces, name)
				}
			}
		} else if !strings.Contains(line, ":") {
			// For space-separated interface names
			for token := range strings.FieldsSeq(line) {
				token = strings.TrimSpace(token)
				if token != "" && !seen[token] {
					seen[token] = true
					ifaces = append(ifaces, token)
				}
			}
		}
	}
	return ifaces
}

// GetRunningWgInterfaces runs `wg` on the remote device and returns all running WireGuard interface names
func GetRunningWgInterfaces(client *ssh.Client) ([]string, error) {
	out, err := RunCommand(client, "wg")
	if err != nil {
		out2, err2 := RunCommand(client, "wg show interfaces")
		if err2 != nil {
			return nil, err
		}
		out = out2
	}
	return ParseWgInterfaces(out), nil
}

// CleanWireGuardInterface brings down the interface using wg-quick down and removes /etc/wireguard/<interface>.conf
func CleanWireGuardInterface(client *ssh.Client, sftpClient *sftp.Client, iface string) error {
	confPath := fmt.Sprintf("/etc/wireguard/%s.conf", iface)

	// 1. Run wg-quick down <interface>
	_, downErr := RunCommand(client, fmt.Sprintf("wg-quick down %s", iface))
	if downErr != nil {
		// Fallback: if wg-quick down failed (e.g. conf file missing), delete ip link directly
		_, _ = RunCommand(client, fmt.Sprintf("ip link del dev %s", iface))
	}

	// 2. Remove /etc/wireguard/<interface>.conf
	if sftpClient != nil {
		_ = sftpClient.Remove(confPath)
	}
	_, rmErr := RunCommand(client, fmt.Sprintf("rm -f %s", confPath))

	if InterfaceExists(client, iface) {
		return fmt.Errorf("interface %s still exists after teardown", iface)
	}
	if downErr != nil && rmErr != nil {
		return fmt.Errorf("failed to clean interface %s: %w", iface, downErr)
	}
	return nil
}

// SyncWireGuard syncs wireguard config dynamically without dropping existing connection
func SyncWireGuard(client *ssh.Client, iface string, confPath string) error {
	// If interface is not up yet, bring it up
	if !InterfaceExists(client, iface) {
		return UpWireGuard(client, iface)
	}

	// Try wg syncconf with stripped config
	cmd := fmt.Sprintf("bash -c 'wg syncconf %s <(wg-quick strip %s)'", iface, confPath)
	_, err := RunCommand(client, cmd)
	if err != nil {
		// Fallback to restart
		_ = DownWireGuard(client, iface)
		return UpWireGuard(client, iface)
	}
	return nil
}

// QueryWireGuardStatus queries wireguard interface statuses using wg show dump
func QueryWireGuardStatus(client *ssh.Client) ([]config.WgInterfaceStatus, error) {
	out, err := RunCommand(client, "wg show all dump")
	if err != nil {
		return nil, err
	}

	var results []config.WgInterfaceStatus
	ifMap := make(map[string]*config.WgInterfaceStatus)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		ifaceName := fields[0]
		if _, exists := ifMap[ifaceName]; !exists {
			ifMap[ifaceName] = &config.WgInterfaceStatus{
				Name:  ifaceName,
				Peers: make([]config.WgPeerStatus, 0),
			}
		}
		status := ifMap[ifaceName]

		// Interface line: <iface> <private-key> <public-key> <listen-port> <fwmark>
		if len(fields) == 5 {
			status.PublicKey = fields[2]
			if port, err := strconv.Atoi(fields[3]); err == nil {
				status.ListenPort = port
			}
		}

		// Peer line: <iface> <public-key> <preshared-key> <endpoint> <allowed-ips> <latest-handshake> <rx-bytes> <tx-bytes> <persistent-keepalive>
		if len(fields) >= 9 {
			var peer config.WgPeerStatus
			peer.PublicKey = fields[1]
			peer.Endpoint = fields[3]
			peer.AllowedIPs = strings.Split(fields[4], ",")
			if hs, err := strconv.ParseInt(fields[5], 10, 64); err == nil && hs > 0 {
				peer.LatestHandshake = time.Unix(hs, 0)
			}
			if rx, err := strconv.ParseInt(fields[6], 10, 64); err == nil {
				peer.TransferRxBytes = rx
			}
			if tx, err := strconv.ParseInt(fields[7], 10, 64); err == nil {
				peer.TransferTxBytes = tx
			}
			if ka, err := strconv.Atoi(fields[8]); err == nil {
				peer.PersistentKeepalive = ka
			}
			status.Peers = append(status.Peers, peer)
		}
	}

	for _, s := range ifMap {
		results = append(results, *s)
	}

	return results, nil
}
