use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use std::fs::{self, File};
use std::io::Write;
use std::net::{SocketAddr, TcpStream, ToSocketAddrs};
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::time::{Duration, Instant};
use tracing::info;

use crate::collector::proto;

#[derive(Serialize, Deserialize, Debug)]
pub struct ProbeConfigFile {
    pub path: String,
    pub content: String,
    pub hash: String,
}

fn is_allowed_config_path(path: &Path) -> bool {
    if !path.is_absolute() {
        return false;
    }
    for comp in path.components() {
        if let std::path::Component::ParentDir = comp {
            return false;
        }
    }
    let p_str = path.to_string_lossy();

    // 1. Exact static configuration files used by easy42
    if p_str == "/etc/easy42.nft"
        || p_str == "/etc/bird_easy42.conf"
        || p_str == "/etc/easy42/agent.json"
    {
        return true;
    }

    // 2. WireGuard interface configs: /etc/wireguard/wg42*.conf
    if let Some(iface) = p_str
        .strip_prefix("/etc/wireguard/")
        .and_then(|s| s.strip_suffix(".conf"))
    {
        if iface.starts_with("wg42") && is_valid_iface_name(iface) {
            return true;
        }
    }

    // 3. Dynamic ROA table configs: /etc/easy42_*_roa4.conf and /etc/easy42_*_roa6.conf
    if let Some(rest) = p_str.strip_prefix("/etc/easy42_") {
        let policy_id = if let Some(id) = rest.strip_suffix("_roa4.conf") {
            Some(id)
        } else {
            rest.strip_suffix("_roa6.conf")
        };
        if let Some(id) = policy_id {
            if !id.is_empty()
                && !id.contains('/')
                && id.chars().all(|c| c.is_ascii_alphanumeric() || c == '_' || c == '-')
            {
                return true;
            }
        }
    }

    // 4. Temporary test directory (for test suites)
    if p_str.starts_with("/tmp/") {
        return true;
    }

    false
}

fn is_valid_iface_name(iface: &str) -> bool {
    !iface.is_empty()
        && iface.len() <= 15
        && iface.chars().all(|c| c.is_ascii_alphanumeric() || c == '_' || c == '-')
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ProbeSystemResult {
    pub hostname: String,
    pub configs: HashMap<String, ProbeConfigFile>,
}

pub struct CommandExecutor;

impl CommandExecutor {
    pub fn new() -> Self {
        Self
    }

    pub async fn execute(&self, cmd: proto::CommandRequest) -> proto::CommandResponse {
        let req_id = cmd.request_id;
        let start = Instant::now();

        let (success, exit_code, stdout, stderr) = match cmd.command {
            Some(proto::command_request::Command::ApplyConfig(apply)) => {
                self.apply_config_file(apply)
            }
            Some(proto::command_request::Command::ManageIface(manage)) => {
                self.manage_interface(manage).await
            }
            Some(proto::command_request::Command::ReloadBird(reload)) => {
                self.reload_bird(reload).await
            }
            Some(proto::command_request::Command::ApplyNft(apply_nft)) => {
                self.apply_nftables(apply_nft).await
            }
            Some(proto::command_request::Command::RestartService(svc)) => {
                self.restart_service(svc).await
            }
            Some(proto::command_request::Command::LookingGlass(lg)) => {
                self.run_looking_glass(lg).await
            }
            Some(proto::command_request::Command::ProbeSystem(_)) => self.probe_system(),
            Some(proto::command_request::Command::FlushCache(_)) => {
                (true, 0, "interface cache flushed".to_string(), String::new())
            }
            None => (false, -1, String::new(), "unknown command".to_string()),
        };

        let duration_ms = start.elapsed().as_millis() as u64;

        proto::CommandResponse {
            request_id: req_id,
            success,
            exit_code,
            stdout,
            stderr,
            duration_ms,
        }
    }

    fn apply_config_file(&self, req: proto::ApplyConfigFile) -> (bool, i32, String, String) {
        // Verify SHA-256 hash if provided
        if !req.sha256_hash.is_empty() {
            let mut hasher = Sha256::new();
            hasher.update(&req.content);
            let calculated = format!("{:x}", hasher.finalize());
            if !calculated.eq_ignore_ascii_case(&req.sha256_hash) {
                return (
                    false,
                    -1,
                    String::new(),
                    format!(
                        "SHA-256 mismatch: expected {}, got {}",
                        req.sha256_hash, calculated
                    ),
                );
            }
        }

        let target_path = Path::new(&req.path);
        if !is_allowed_config_path(target_path) {
            return (
                false,
                -1,
                String::new(),
                format!("path '{}' not allowed by agent security policy", req.path),
            );
        }

        if let Some(parent) = target_path.parent() {
            if let Err(e) = fs::create_dir_all(parent) {
                return (
                    false,
                    -1,
                    String::new(),
                    format!("failed to create dir {:?}: {}", parent, e),
                );
            }
            // Ensure parent directories have 0755 permissions
            let mut curr = Some(parent);
            while let Some(dir) = curr {
                if dir == Path::new("/") || dir == Path::new("/etc") || dir.as_os_str().is_empty() {
                    break;
                }
                let _ = fs::set_permissions(dir, fs::Permissions::from_mode(0o755));
                curr = dir.parent();
            }
        }

        let tmp_path = format!("{}.tmp.{}", req.path, std::process::id());
        let mode = if req.path.ends_with(".nft") || req.path.ends_with(".sh") {
            0o755
        } else if req.file_mode != 0 {
            req.file_mode
        } else {
            0o644
        };

        match File::create(&tmp_path) {
            Ok(mut file) => {
                if let Err(e) = file.write_all(&req.content) {
                    let _ = fs::remove_file(&tmp_path);
                    return (
                        false,
                        -1,
                        String::new(),
                        format!("failed to write tmp file: {}", e),
                    );
                }
                if let Err(e) = file.sync_all() {
                    let _ = fs::remove_file(&tmp_path);
                    return (
                        false,
                        -1,
                        String::new(),
                        format!("failed to sync tmp file: {}", e),
                    );
                }
                let _ = fs::set_permissions(&tmp_path, fs::Permissions::from_mode(mode));

                if let Err(e) = fs::rename(&tmp_path, &req.path) {
                    let _ = fs::remove_file(&tmp_path);
                    return (
                        false,
                        -1,
                        String::new(),
                        format!("failed to atomic rename to {}: {}", req.path, e),
                    );
                }
                let _ = fs::set_permissions(&req.path, fs::Permissions::from_mode(mode));
                info!("Successfully wrote file {} (mode {:o})", req.path, mode);
                (true, 0, format!("wrote {}", req.path), String::new())
            }
            Err(e) => (
                false,
                -1,
                String::new(),
                format!("failed to create tmp file {}: {}", tmp_path, e),
            ),
        }
    }

    async fn manage_interface(&self, req: proto::ManageInterface) -> (bool, i32, String, String) {
        let action = match proto::manage_interface::Action::try_from(req.action) {
            Ok(a) => a,
            Err(_) => {
                return (
                    false,
                    -1,
                    String::new(),
                    "invalid interface action".to_string(),
                )
            }
        };

        let iface = req.interface_name;
        if !is_valid_iface_name(&iface) {
            return (
                false,
                -1,
                String::new(),
                format!("invalid interface name: {}", iface),
            );
        }

        match action {
            proto::manage_interface::Action::Up => {
                let res = run_cmd("wg-quick", &["up", &iface]).await;
                if res.0 {
                    res
                } else {
                    let check = run_cmd("ip", &["link", "show", &iface]).await;
                    if check.0 {
                        (true, 0, res.2, res.3)
                    } else {
                        res
                    }
                }
            }
            proto::manage_interface::Action::Down => {
                let res = run_cmd("wg-quick", &["down", &iface]).await;
                if res.0 {
                    res
                } else {
                    let ip_del = run_cmd("ip", &["link", "del", "dev", &iface]).await;
                    if ip_del.0 {
                        (true, 0, ip_del.2, ip_del.3)
                    } else {
                        res
                    }
                }
            }
            proto::manage_interface::Action::Restart => {
                let _ = run_cmd("wg-quick", &["down", &iface]).await;
                let _ = run_cmd("ip", &["link", "del", "dev", &iface]).await;
                run_cmd("wg-quick", &["up", &iface]).await
            }
            proto::manage_interface::Action::SyncWg => {
                let conf_file = if req.config_file.is_empty() {
                    format!("/etc/wireguard/{}.conf", iface)
                } else {
                    req.config_file
                };
                let conf_path = Path::new(&conf_file);
                if !is_allowed_config_path(conf_path) {
                    return (
                        false,
                        -1,
                        String::new(),
                        format!("config path '{}' not allowed by agent security policy", conf_file),
                    );
                }
                let check_up = tokio::process::Command::new("ip").args(["link", "show", &iface]).output().await;
                let is_up = check_up.map(|o| o.status.success()).unwrap_or(false);
                if is_up {
                    run_shell(&format!(
                        "(wg-quick strip \"{conf}\" | wg syncconf \"{iface}\" /dev/stdin) 2>&1 || (wg-quick down \"{iface}\" 2>/dev/null; wg-quick up \"{iface}\")",
                        conf = conf_file,
                        iface = iface
                    )).await
                } else {
                    run_cmd("wg-quick", &["up", &iface]).await
                }
            }
            proto::manage_interface::Action::Delete => {
                let _ = run_cmd("wg-quick", &["down", &iface]).await;
                let _ = run_cmd("ip", &["link", "del", "dev", &iface]).await;
                let conf_path = format!("/etc/wireguard/{}.conf", iface);
                let _ = std::fs::remove_file(&conf_path);
                (true, 0, format!("deleted interface {}", iface), String::new())
            }
        }
    }

    async fn reload_bird(&self, req: proto::ReloadBirdCmd) -> (bool, i32, String, String) {
        if req.check_only {
            run_shell("bird -p").await
        } else {
            run_shell("birdc configure").await
        }
    }

    async fn apply_nftables(&self, req: proto::ApplyNftablesCmd) -> (bool, i32, String, String) {
        let script = if req.script_path.is_empty() {
            "/etc/easy42.nft"
        } else {
            &req.script_path
        };
        let script_path = Path::new(script);
        if !is_allowed_config_path(script_path) {
            return (
                false,
                -1,
                String::new(),
                format!("script path '{}' not allowed by agent security policy", script),
            );
        }
        if let Ok(metadata) = fs::metadata(script_path) {
            let mut perms = metadata.permissions();
            perms.set_mode(0o755);
            let _ = fs::set_permissions(script_path, perms);
        }
        run_cmd(script, &[]).await
    }

    async fn restart_service(&self, req: proto::RestartServiceCmd) -> (bool, i32, String, String) {
        match req.service_name.as_str() {
            "bird" => {
                run_shell("systemctl restart bird 2>/dev/null || service bird restart 2>&1").await
            }
            "wireguard" => {
                let cmd = r#"for i in $( { for c in /etc/wireguard/*.conf; do [ -f "$c" ] && { name="${c##*/}"; printf "%s\n" "${name%.conf}"; }; done; wg show interfaces 2>/dev/null | tr " " "\n"; } | sort -u ); do
  [ -n "$i" ] || continue
  wg-quick down "$i" 2>/dev/null || ip link del dev "$i" 2>/dev/null
  [ -f "/etc/wireguard/$i.conf" ] && wg-quick up "$i"
done"#;
                run_shell(cmd).await
            }
            other => (
                false,
                -1,
                String::new(),
                format!("unsupported service restart: {}", other),
            ),
        }
    }

    async fn run_looking_glass(&self, req: proto::LookingGlassCmd) -> (bool, i32, String, String) {
        let tool = match proto::looking_glass_cmd::Tool::try_from(req.tool) {
            Ok(t) => t,
            Err(_) => proto::looking_glass_cmd::Tool::Custom,
        };

        match tool {
            proto::looking_glass_cmd::Tool::Ping => {
                let count = req.args.first().cloned().unwrap_or_else(|| "4".to_string());
                run_cmd("ping", &["-c", &count, &req.target]).await
            }
            proto::looking_glass_cmd::Tool::Traceroute => {
                run_cmd("traceroute", &[&req.target]).await
            }
            proto::looking_glass_cmd::Tool::Birdc => {
                let mut full_args = vec!["show"];
                let args_ref: Vec<&str> = req.args.iter().map(|s| s.as_str()).collect();
                full_args.extend(args_ref);
                run_cmd("birdc", &full_args).await
            }
            proto::looking_glass_cmd::Tool::Custom => {
                if req.command.is_empty() {
                    return (false, -1, String::new(), "empty custom command".to_string());
                }
                // Reject dangerous shell metacharacters
                if req.command.chars().any(|c| matches!(c, ';' | '&' | '|' | '`' | '$' | '\n' | '\r' | '>' | '<')) {
                    return (
                        false,
                        -1,
                        String::new(),
                        "command contains disallowed characters".to_string(),
                    );
                }
                let words: Vec<&str> = req.command.split_whitespace().collect();
                if words.is_empty() {
                    return (false, -1, String::new(), "empty custom command".to_string());
                }
                let program = words[0];
                match program {
                    "ping" | "traceroute" | "birdc" | "ip" | "wg" | "mtr" | "whois" | "dig" => {
                        run_cmd(program, &words[1..]).await
                    }
                    _ => (
                        false,
                        -1,
                        String::new(),
                        format!(
                            "command '{}' not allowed by agent security policy",
                            program
                        ),
                    ),
                }
            }
            proto::looking_glass_cmd::Tool::TcpPing => {
                // Native TCP ping: dial target, measure RTT, return JSON
                let target = req.target.clone();
                let timeout_secs = if req.timeout_seconds > 0 { req.timeout_seconds } else { 10 };
                // Repeat count from args[0], default 4
                let count: u32 = req.args.first()
                    .and_then(|s| s.parse().ok())
                    .unwrap_or(4)
                    .min(20);

                run_tcp_ping(&target, count, timeout_secs).await
            }
        }
    }

    fn probe_system(&self) -> (bool, i32, String, String) {
        let hostname = fs::read_to_string("/proc/sys/kernel/hostname")
            .unwrap_or_default()
            .trim()
            .to_string();

        let mut configs = HashMap::new();
        if let Ok(entries) = fs::read_dir("/etc/wireguard") {
            for entry in entries.flatten() {
                let path = entry.path();
                if let Some(file_name) = path.file_name().and_then(|n| n.to_str()) {
                    if file_name.starts_with("wg42") && file_name.ends_with(".conf") {
                        let iface = file_name.trim_end_matches(".conf").to_string();
                        if let Ok(content) = fs::read_to_string(&path) {
                            let mut hasher = Sha256::new();
                            let normalized = content.replace("\r\n", "\n");
                            let normalized_trimmed = normalized.trim_end();
                            hasher.update(normalized_trimmed.as_bytes());
                            let hash = format!("{:x}", hasher.finalize());

                            configs.insert(
                                iface,
                                ProbeConfigFile {
                                    path: path.to_string_lossy().to_string(),
                                    content,
                                    hash,
                                },
                            );
                        }
                    }
                }
            }
        }

        let result = ProbeSystemResult { hostname, configs };

        match serde_json::to_string(&result) {
            Ok(json) => (true, 0, json, String::new()),
            Err(e) => (
                false,
                -1,
                String::new(),
                format!("failed to serialize probe result: {}", e),
            ),
        }
    }
}

/// run_tcp_ping connects to `host:port` `count` times, measuring TCP handshake RTT each time.
/// Returns JSON-encoded TCPPingResult on stdout, or an error message on stderr.
pub(crate) async fn run_tcp_ping(target: &str, count: u32, timeout_secs: u32) -> (bool, i32, String, String) {
    // Parse "host:port" or "[ipv6]:port"
    let addrs: Vec<SocketAddr> = match target.to_socket_addrs() {
        Ok(iter) => iter.collect(),
        Err(e) => {
            return (
                false,
                -1,
                String::new(),
                format!("failed to resolve '{}': {}", target, e),
            )
        }
    };
    if addrs.is_empty() {
        return (
            false,
            -1,
            String::new(),
            format!("no addresses resolved for '{}'", target),
        );
    }
    let addr = addrs[0];
    let timeout = Duration::from_secs(timeout_secs as u64);

    let mut rtts: Vec<f64> = Vec::new();
    let mut failures: u32 = 0;

    for _ in 0..count {
        let addr_clone = addr;
        let timeout_clone = timeout;
        let probe_start = Instant::now();
        let result = tokio::task::spawn_blocking(move || {
            TcpStream::connect_timeout(&addr_clone, timeout_clone)
        })
        .await;

        match result {
            Ok(Ok(_stream)) => {
                let rtt_ms = probe_start.elapsed().as_secs_f64() * 1000.0;
                rtts.push(rtt_ms);
            }
            _ => {
                failures += 1;
            }
        }

        // Small pause between probes to avoid overwhelming the target
        if count > 1 {
            tokio::time::sleep(Duration::from_millis(200)).await;
        }
    }

    let packets_sent = count;
    let packets_received = rtts.len() as u32;
    let loss_pct = if packets_sent > 0 {
        (failures as f64 / packets_sent as f64) * 100.0
    } else {
        100.0
    };

    let (min_rtt, max_rtt, avg_rtt, mdev_rtt) = if !rtts.is_empty() {
        let min = rtts.iter().cloned().fold(f64::INFINITY, f64::min);
        let max = rtts.iter().cloned().fold(f64::NEG_INFINITY, f64::max);
        let avg = rtts.iter().sum::<f64>() / rtts.len() as f64;
        let variance = rtts.iter().map(|&r| (r - avg).powi(2)).sum::<f64>() / rtts.len() as f64;
        let mdev = variance.sqrt();
        (min, max, avg, mdev)
    } else {
        (0.0, 0.0, 0.0, 0.0)
    };

    // Build structured JSON output matching TCPPingResult on the server side
    let json = format!(
        r#"{{"target":"{target}","resolved_addr":"{addr}","packets_sent":{ps},"packets_received":{pr},"packet_loss_pct":{loss:.2},"min_rtt_ms":{min:.3},"avg_rtt_ms":{avg:.3},"max_rtt_ms":{max:.3},"mdev_rtt_ms":{mdev:.3},"rtts":[{rtts_json}]}}"#,
        target = target,
        addr = addr,
        ps = packets_sent,
        pr = packets_received,
        loss = loss_pct,
        min = min_rtt,
        avg = avg_rtt,
        max = max_rtt,
        mdev = mdev_rtt,
        rtts_json = rtts.iter().map(|r| format!("{:.3}", r)).collect::<Vec<_>>().join(","),
    );

    let success = packets_received > 0;
    let exit_code = if success { 0 } else { 1 };
    (success, exit_code, json, String::new())
}

pub(crate) async fn run_cmd(program: &str, args: &[&str]) -> (bool, i32, String, String) {
    use tokio::process::Command;
    let mut cmd = Command::new(program);
    cmd.args(args);
    match tokio::time::timeout(std::time::Duration::from_secs(30), cmd.output()).await {
        Ok(Ok(output)) => {
            let code = output.status.code().unwrap_or(-1);
            let stdout = String::from_utf8_lossy(&output.stdout).to_string();
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            (output.status.success(), code, stdout, stderr)
        }
        Ok(Err(e)) => (
            false,
            -1,
            String::new(),
            format!("failed to execute {}: {}", program, e),
        ),
        Err(_) => (
            false,
            -1,
            String::new(),
            format!("command {} timed out after 30s", program),
        ),
    }
}

async fn run_shell(cmd_str: &str) -> (bool, i32, String, String) {
    use tokio::process::Command;
    let mut cmd = Command::new("sh");
    cmd.arg("-c").arg(cmd_str);
    match tokio::time::timeout(std::time::Duration::from_secs(30), cmd.output()).await {
        Ok(Ok(output)) => {
            let code = output.status.code().unwrap_or(-1);
            let stdout = String::from_utf8_lossy(&output.stdout).to_string();
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            (output.status.success(), code, stdout, stderr)
        }
        Ok(Err(e)) => (false, -1, String::new(), format!("failed to run sh: {}", e)),
        Err(_) => (
            false,
            -1,
            String::new(),
            "command timed out after 30s".to_string(),
        ),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;

    #[test]
    fn test_allowed_config_paths() {
        // Valid exact paths
        assert!(is_allowed_config_path(Path::new("/etc/easy42.nft")));
        assert!(is_allowed_config_path(Path::new("/etc/bird_easy42.conf")));
        assert!(is_allowed_config_path(Path::new("/etc/easy42/agent.json")));

        // Valid WireGuard configs
        assert!(is_allowed_config_path(Path::new("/etc/wireguard/wg42_node1.conf")));
        assert!(is_allowed_config_path(Path::new("/etc/wireguard/wg42.conf")));

        // Valid ROA configs
        assert!(is_allowed_config_path(Path::new("/etc/easy42_policy1_roa4.conf")));
        assert!(is_allowed_config_path(Path::new("/etc/easy42_policy1_roa6.conf")));

        // Valid test paths
        assert!(is_allowed_config_path(Path::new("/tmp/test.conf")));
        assert!(is_allowed_config_path(Path::new("/tmp/sub/dir/test.txt")));

        // Invalid paths - loose prefixes that must be rejected
        assert!(!is_allowed_config_path(Path::new("/etc/wireguard/other.conf")));
        assert!(!is_allowed_config_path(Path::new("/etc/wireguard/wg42_toolongname12345.conf")));
        assert!(!is_allowed_config_path(Path::new("/etc/bird/bird.conf")));
        assert!(!is_allowed_config_path(Path::new("/etc/bird.conf")));
        assert!(!is_allowed_config_path(Path::new("/etc/nftables.conf")));
        assert!(!is_allowed_config_path(Path::new("/etc/easy42/evil.sh")));
        assert!(!is_allowed_config_path(Path::new("/etc/cron.d/backdoor")));
        assert!(!is_allowed_config_path(Path::new("/root/.ssh/authorized_keys")));
        assert!(!is_allowed_config_path(Path::new("/etc/sudoers.d/easy42")));

        // Path traversal attempts
        assert!(!is_allowed_config_path(Path::new("/etc/wireguard/../cron.d/backdoor")));
        assert!(!is_allowed_config_path(Path::new("/tmp/../etc/shadow")));

        // Relative paths
        assert!(!is_allowed_config_path(Path::new("etc/easy42.nft")));
    }

    #[test]
    fn test_valid_iface_name() {
        assert!(is_valid_iface_name("wg42"));
        assert!(is_valid_iface_name("wg42_a_b"));
        assert!(is_valid_iface_name("wg-0"));
        assert!(is_valid_iface_name("eth0"));

        assert!(!is_valid_iface_name(""));
        assert!(!is_valid_iface_name("this_name_is_way_too_long_for_ifname"));
        assert!(!is_valid_iface_name("wg42;rm -rf"));
        assert!(!is_valid_iface_name("wg 42"));
        assert!(!is_valid_iface_name("wg$foo"));
    }
}

