use std::collections::HashMap;
use std::fs::{self, File};
use std::io::Write;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::process::Command;
use std::time::Instant;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use tracing::info;

use crate::collector::proto;

#[derive(Serialize, Deserialize, Debug)]
pub struct ProbeConfigFile {
    pub path: String,
    pub content: String,
    pub hash: String,
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

    pub fn execute(&self, cmd: proto::CommandRequest) -> proto::CommandResponse {
        let req_id = cmd.request_id;
        let start = Instant::now();

        let (success, exit_code, stdout, stderr) = match cmd.command {
            Some(proto::command_request::Command::ApplyConfig(apply)) => {
                self.apply_config_file(apply)
            }
            Some(proto::command_request::Command::ManageIface(manage)) => {
                self.manage_interface(manage)
            }
            Some(proto::command_request::Command::ReloadBird(reload)) => {
                self.reload_bird(reload)
            }
            Some(proto::command_request::Command::ApplyNft(apply_nft)) => {
                self.apply_nftables(apply_nft)
            }
            Some(proto::command_request::Command::RestartService(svc)) => {
                self.restart_service(svc)
            }
            Some(proto::command_request::Command::LookingGlass(lg)) => {
                self.run_looking_glass(lg)
            }
            Some(proto::command_request::Command::ProbeSystem(_)) => {
                self.probe_system()
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
                    format!("SHA-256 mismatch: expected {}, got {}", req.sha256_hash, calculated),
                );
            }
        }

        let target_path = Path::new(&req.path);
        if let Some(parent) = target_path.parent() {
            if let Err(e) = fs::create_dir_all(parent) {
                return (false, -1, String::new(), format!("failed to create dir {:?}: {}", parent, e));
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
                    return (false, -1, String::new(), format!("failed to write tmp file: {}", e));
                }
                if let Err(e) = file.sync_all() {
                    let _ = fs::remove_file(&tmp_path);
                    return (false, -1, String::new(), format!("failed to sync tmp file: {}", e));
                }
                let _ = fs::set_permissions(&tmp_path, fs::Permissions::from_mode(mode));

                if let Err(e) = fs::rename(&tmp_path, &req.path) {
                    let _ = fs::remove_file(&tmp_path);
                    return (false, -1, String::new(), format!("failed to atomic rename to {}: {}", req.path, e));
                }
                let _ = fs::set_permissions(&req.path, fs::Permissions::from_mode(mode));
                info!("Successfully wrote file {} (mode {:o})", req.path, mode);
                (true, 0, format!("wrote {}", req.path), String::new())
            }
            Err(e) => (false, -1, String::new(), format!("failed to create tmp file {}: {}", tmp_path, e)),
        }
    }

    fn manage_interface(&self, req: proto::ManageInterface) -> (bool, i32, String, String) {
        let action = match proto::manage_interface::Action::try_from(req.action) {
            Ok(a) => a,
            Err(_) => return (false, -1, String::new(), "invalid interface action".to_string()),
        };

        let iface = req.interface_name;

        match action {
            proto::manage_interface::Action::Up => {
                run_shell(&format!("wg-quick up {} 2>&1 || true", iface))
            }
            proto::manage_interface::Action::Down => {
                run_shell(&format!("wg-quick down {} 2>&1 || ip link del dev {} 2>&1 || true", iface, iface))
            }
            proto::manage_interface::Action::Restart => {
                run_shell(&format!("wg-quick down {} 2>/dev/null || ip link del dev {} 2>/dev/null; wg-quick up {}", iface, iface, iface))
            }
            proto::manage_interface::Action::SyncWg => {
                let conf_file = if req.config_file.is_empty() {
                    format!("/etc/wireguard/{}.conf", iface)
                } else {
                    req.config_file
                };
                // If interface not up, bring it up; if already up, run wg syncconf
                let check_up = Command::new("ip").args(["link", "show", &iface]).output();
                let is_up = check_up.map(|o| o.status.success()).unwrap_or(false);
                if is_up {
                    run_shell(&format!("wg syncconf {} <(wg-quick strip {}) 2>&1 || wg-quick down {} 2>/dev/null; wg-quick up {}", iface, conf_file, iface, iface))
                } else {
                    run_shell(&format!("wg-quick up {} 2>&1", iface))
                }
            }
            proto::manage_interface::Action::Delete => {
                run_shell(&format!("wg-quick down {} 2>/dev/null || ip link del dev {} 2>/dev/null; rm -f /etc/wireguard/{}.conf", iface, iface, iface))
            }
        }
    }

    fn reload_bird(&self, req: proto::ReloadBirdCmd) -> (bool, i32, String, String) {
        if req.check_only {
            run_shell("bird -p")
        } else {
            run_shell("birdc configure")
        }
    }

    fn apply_nftables(&self, req: proto::ApplyNftablesCmd) -> (bool, i32, String, String) {
        let script = if req.script_path.is_empty() {
            "/etc/easy42.nft"
        } else {
            &req.script_path
        };
        run_shell(&format!("chmod +x {} 2>/dev/null; {}", script, script))
    }

    fn restart_service(&self, req: proto::RestartServiceCmd) -> (bool, i32, String, String) {
        match req.service_name.as_str() {
            "bird" => {
                run_shell("systemctl restart bird 2>/dev/null || service bird restart 2>&1")
            }
            "wireguard" => {
                let cmd = r#"sh -c '
for i in $( { for c in /etc/wireguard/*.conf; do [ -f "$c" ] && basename "$c" .conf; done; wg show interfaces 2>/dev/null | tr " " "\n"; } | sort -u ); do
  [ -n "$i" ] || continue
  wg-quick down "$i" 2>/dev/null || ip link del dev "$i" 2>/dev/null
  [ -f "/etc/wireguard/$i.conf" ] && wg-quick up "$i"
done
'"#;
                run_shell(cmd)
            }
            other => (false, -1, String::new(), format!("unsupported service restart: {}", other)),
        }
    }

    fn run_looking_glass(&self, req: proto::LookingGlassCmd) -> (bool, i32, String, String) {
        let tool = match proto::looking_glass_cmd::Tool::try_from(req.tool) {
            Ok(t) => t,
            Err(_) => proto::looking_glass_cmd::Tool::Custom,
        };

        match tool {
            proto::looking_glass_cmd::Tool::Ping => {
                let count = req.args.first().cloned().unwrap_or_else(|| "4".to_string());
                run_cmd("ping", &["-c", &count, &req.target])
            }
            proto::looking_glass_cmd::Tool::Traceroute => {
                run_cmd("traceroute", &[&req.target])
            }
            proto::looking_glass_cmd::Tool::Birdc => {
                let mut full_args = vec!["show"];
                let args_ref: Vec<&str> = req.args.iter().map(|s| s.as_str()).collect();
                full_args.extend(args_ref);
                run_cmd("birdc", &full_args)
            }
            proto::looking_glass_cmd::Tool::Custom => {
                if req.command.is_empty() {
                    return (false, -1, String::new(), "empty custom command".to_string());
                }
                // Sanitize command: restrict to diagnostics (ping, traceroute, birdc, wg, ip)
                let first_word = req.command.split_whitespace().next().unwrap_or("");
                match first_word {
                    "ping" | "traceroute" | "birdc" | "ip" | "wg" => run_shell(&req.command),
                    _ => (false, -1, String::new(), format!("command '{}' not allowed by agent security policy", first_word)),
                }
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

        let result = ProbeSystemResult {
            hostname,
            configs,
        };

        match serde_json::to_string(&result) {
            Ok(json) => (true, 0, json, String::new()),
            Err(e) => (false, -1, String::new(), format!("failed to serialize probe result: {}", e)),
        }
    }
}

fn run_cmd(program: &str, args: &[&str]) -> (bool, i32, String, String) {
    match Command::new(program).args(args).output() {
        Ok(output) => {
            let code = output.status.code().unwrap_or(-1);
            let stdout = String::from_utf8_lossy(&output.stdout).to_string();
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            (output.status.success(), code, stdout, stderr)
        }
        Err(e) => (false, -1, String::new(), format!("failed to execute {}: {}", program, e)),
    }
}

fn run_shell(cmd_str: &str) -> (bool, i32, String, String) {
    match Command::new("bash").arg("-c").arg(cmd_str).output() {
        Ok(output) => {
            let code = output.status.code().unwrap_or(-1);
            let stdout = String::from_utf8_lossy(&output.stdout).to_string();
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            (output.status.success(), code, stdout, stderr)
        }
        Err(e) => (false, -1, String::new(), format!("failed to run bash: {}", e)),
    }
}
