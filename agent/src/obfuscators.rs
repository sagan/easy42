use std::collections::HashSet;
use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use tracing::{error, info};

use crate::apps::{detect_init_system, InitSystem, OPENRC_INIT_DIR, PROCD_INIT_DIR, SYSTEMD_SYSTEM_DIR};
use crate::collector::proto;
use crate::executor::run_shell;

const OBFUSCATOR_SUFFIX: &str = "-obfuscator";

fn is_safe_token(s: &str) -> bool {
    !s.is_empty()
        && s.len() <= 512
        && !s
            .chars()
            .any(|c| c.is_whitespace() || matches!(c, ';' | '|' | '&' | '$' | '<' | '>' | '`' | '"' | '\'' | '\\' | '(' | ')' | '{' | '}' | '[' | ']' | '*' | '?' | '!' | '#' | '~'))
}

fn shell_quote(s: &str) -> String {
    if !s.is_empty()
        && s.chars()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, '_' | '-' | '.' | '/' | ':' | '=' | ',' | '@' | '+' | '%'))
    {
        return s.to_string();
    }
    format!("'{}'", s.replace('\'', "'\\''"))
}

fn valid_service_name(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 64
        && name.starts_with("wg42")
        && name.ends_with(OBFUSCATOR_SUFFIX)
        && name
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, '_' | '-' | '.'))
}

fn resolve_binary(name: &str) -> String {
    if let Ok(output) = std::process::Command::new("sh")
        .arg("-c")
        .arg(format!("command -v {} 2>/dev/null", name))
        .output()
    {
        if output.status.success() {
            let path = String::from_utf8_lossy(&output.stdout).trim().to_string();
            if path.starts_with('/') && !path.contains(char::is_whitespace) {
                return path;
            }
        }
    }
    // Best effort fallback; the service will report a clear failure if absent.
    format!("/usr/bin/{}", name)
}

fn build_args(spec: &proto::ObfuscatorSpec, binary: &str) -> Result<Vec<String>, String> {
    if !valid_service_name(&spec.service_name) {
        return Err(format!("invalid obfuscator service name '{}'", spec.service_name));
    }
    if spec.tun_name.is_empty() || spec.tun_name.len() > 15 || !is_safe_token(&spec.tun_name) {
        return Err(format!("invalid obfuscator TUN name '{}'", spec.tun_name));
    }
    for (label, value) in [
        ("tun_local", spec.tun_local.as_str()),
        ("tun_peer", spec.tun_peer.as_str()),
        ("remote", spec.remote.as_str()),
        ("fwmark", spec.fwmark.as_str()),
    ] {
        if !value.is_empty() && !is_safe_token(value) {
            return Err(format!("invalid obfuscator {} value '{}'", label, value));
        }
    }

    let mut args = vec![binary.to_string(), "-4".to_string()];
    match spec.role.as_str() {
        "phantun_client" => {
            if spec.local_port <= 0 || spec.local_port > 65535 {
                return Err("invalid obfuscator local_port".to_string());
            }
            if spec.remote.is_empty() {
                return Err("missing obfuscator remote endpoint".to_string());
            }
            args.push("--local".to_string());
            args.push(format!("0.0.0.0:{}", spec.local_port));
            args.push("--remote".to_string());
            args.push(spec.remote.clone());
            args.push("--tun".to_string());
            args.push(spec.tun_name.clone());
            args.push("--tun-local".to_string());
            args.push(spec.tun_local.clone());
            args.push("--tun-peer".to_string());
            args.push(spec.tun_peer.clone());
            args.push("--keepalive-time".to_string());
            args.push("15".to_string());
            if !spec.fwmark.is_empty() {
                args.push("--fwmark".to_string());
                args.push(spec.fwmark.clone());
            }
        }
        "phantun_server" => {
            if spec.local_port <= 0 || spec.local_port > 65535 {
                return Err("invalid obfuscator local_port".to_string());
            }
            if spec.wireguard_port <= 0 || spec.wireguard_port > 65535 {
                return Err("invalid obfuscator wireguard_port".to_string());
            }
            args.push("--local".to_string());
            args.push(format!("{}", spec.local_port));
            args.push("--remote".to_string());
            args.push(format!("127.0.0.1:{}", spec.wireguard_port));
            args.push("--tun".to_string());
            args.push(spec.tun_name.clone());
            args.push("--tun-local".to_string());
            args.push(spec.tun_local.clone());
            args.push("--tun-peer".to_string());
            args.push(spec.tun_peer.clone());
        }
        other => return Err(format!("unknown obfuscator role '{}'", other)),
    }

    for extra in &spec.extra_args {
        if !is_safe_token(extra) {
            return Err(format!("unsafe extra obfuscator argument '{}'", extra));
        }
        args.push(extra.clone());
    }
    Ok(args)
}

fn systemd_unit(spec: &proto::ObfuscatorSpec, args: &[String]) -> String {
    let command = args.iter().map(|a| shell_quote(a)).collect::<Vec<_>>().join(" ");
    format!(
        "[Unit]\nDescription=Easy42 WireGuard obfuscator for {iface}\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nEnvironment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\nExecStart={command}\nRestart=always\nRestartSec=3\n\n[Install]\nWantedBy=multi-user.target\n",
        iface = spec.interface_name,
        command = command
    )
}

fn openrc_script(spec: &proto::ObfuscatorSpec, args: &[String]) -> String {
    let command = shell_quote(&args[0]);
    let command_args = args[1..].iter().map(|a| shell_quote(a)).collect::<Vec<_>>().join(" ");
    format!(
        "#!/sbin/openrc-run\n# Generated by Easy42 for {iface}\ndescription=\"Easy42 WireGuard obfuscator for {iface}\"\ncommand={command}\ncommand_args=\"{command_args}\"\ncommand_background=true\npidfile=\"/run/${{RC_SVCNAME}}.pid\"\n\nrespawn_delay=3\nrespawn_max=0\n\ndepend() {{\n    need net\n    after firewall\n}}\n",
        iface = spec.interface_name,
        command = command,
        command_args = command_args
    )
}

fn procd_script(spec: &proto::ObfuscatorSpec, args: &[String]) -> String {
    let command = args.iter().map(|a| shell_quote(a)).collect::<Vec<_>>().join(" ");
    format!(
        "#!/bin/sh /etc/rc.common\n# Generated by Easy42 for {iface}\nUSE_PROCD=1\nSTART=95\nSTOP=10\n\nstart_service() {{\n    procd_open_instance\n    procd_set_param command {command}\n    procd_set_param respawn 3600 5 5\n    procd_set_param stdout 1\n    procd_set_param stderr 1\n    procd_close_instance\n}}\n",
        iface = spec.interface_name,
        command = command
    )
}

async fn apply_spec(init_system: InitSystem, spec: &proto::ObfuscatorSpec) -> Result<(), String> {
    if !spec.enabled {
        return Ok(());
    }
    let binary_name = if spec.role == "phantun_server" {
        "phantun_server"
    } else {
        "phantun_client"
    };
    let binary = resolve_binary(binary_name);
    let args = build_args(spec, &binary)?;
    let service_name = spec.service_name.clone();

    match init_system {
        InitSystem::Systemd => {
            let path = Path::new(SYSTEMD_SYSTEM_DIR).join(format!("{}.service", service_name));
            let content = systemd_unit(spec, &args);
            let changed = fs::read_to_string(&path).map(|old| old != content).unwrap_or(true);
            if changed {
                fs::write(&path, &content)
                    .map_err(|e| format!("failed to write systemd unit {}: {}", path.display(), e))?;
                info!("Installed systemd obfuscator unit {}", path.display());
            }
            let _ = run_shell("systemctl daemon-reload").await;
            let (ok, code, stdout, stderr) =
                run_shell(&format!("systemctl enable --now {}", shell_quote(&service_name))).await;
            if !ok {
                return Err(format!(
                    "systemctl enable --now {} failed (code {}): {} {}",
                    service_name, code, stdout, stderr
                ));
            }
            if changed {
                let _ = run_shell(&format!("systemctl restart {}", shell_quote(&service_name))).await;
            }
        }
        InitSystem::OpenRC => {
            let path = Path::new(OPENRC_INIT_DIR).join(&service_name);
            let content = openrc_script(spec, &args);
            let changed = fs::read_to_string(&path).map(|old| old != content).unwrap_or(true);
            if changed {
                fs::write(&path, &content)
                    .map_err(|e| format!("failed to write OpenRC script {}: {}", path.display(), e))?;
                let _ = fs::set_permissions(&path, fs::Permissions::from_mode(0o755));
                info!("Installed OpenRC obfuscator script {}", path.display());
            }
            let _ = run_shell(&format!("rc-update add {} default 2>/dev/null", shell_quote(&service_name))).await;
            let action = if changed { "restart" } else { "start" };
            let (ok, code, stdout, stderr) =
                run_shell(&format!("rc-service {} {}", shell_quote(&service_name), action)).await;
            if !ok && action == "restart" {
                return Err(format!(
                    "rc-service {} restart failed (code {}): {} {}",
                    service_name, code, stdout, stderr
                ));
            }
        }
        InitSystem::Procd => {
            let path = Path::new(PROCD_INIT_DIR).join(&service_name);
            let content = procd_script(spec, &args);
            let changed = fs::read_to_string(&path).map(|old| old != content).unwrap_or(true);
            if changed {
                fs::write(&path, &content)
                    .map_err(|e| format!("failed to write Procd script {}: {}", path.display(), e))?;
                let _ = fs::set_permissions(&path, fs::Permissions::from_mode(0o755));
                info!("Installed Procd obfuscator script {}", path.display());
            }
            let _ = run_shell(&format!("/etc/init.d/{} enable 2>/dev/null", shell_quote(&service_name))).await;
            let action = if changed { "restart" } else { "start" };
            let (ok, code, stdout, stderr) =
                run_shell(&format!("/etc/init.d/{} {}", shell_quote(&service_name), action)).await;
            if !ok && action == "restart" {
                return Err(format!(
                    "/etc/init.d/{} restart failed (code {}): {} {}",
                    service_name, code, stdout, stderr
                ));
            }
        }
    }
    Ok(())
}

async fn cleanup_orphans(init_system: InitSystem, desired: &HashSet<String>) {
    match init_system {
        InitSystem::Systemd => {
            if let Ok(entries) = fs::read_dir(SYSTEMD_SYSTEM_DIR) {
                for entry in entries.flatten() {
                    let file_name = entry.file_name().to_string_lossy().to_string();
                    if !file_name.starts_with("wg42") || !file_name.ends_with(&format!("{}.service", OBFUSCATOR_SUFFIX)) {
                        continue;
                    }
                    let name = file_name.trim_end_matches(".service").to_string();
                    if desired.contains(&name) {
                        continue;
                    }
                    info!("Removing orphaned systemd obfuscator unit {}", name);
                    let _ = run_shell(&format!("systemctl stop {}", shell_quote(&name))).await;
                    let _ = run_shell(&format!("systemctl disable {}", shell_quote(&name))).await;
                    let _ = fs::remove_file(entry.path());
                }
            }
            let _ = run_shell("systemctl daemon-reload").await;
        }
        InitSystem::OpenRC | InitSystem::Procd => {
            if let Ok(entries) = fs::read_dir(OPENRC_INIT_DIR) {
                for entry in entries.flatten() {
                    let name = entry.file_name().to_string_lossy().to_string();
                    if !name.starts_with("wg42") || !name.ends_with(OBFUSCATOR_SUFFIX) {
                        continue;
                    }
                    if desired.contains(&name) {
                        continue;
                    }
                    info!("Removing orphaned obfuscator service {}", name);
                    match init_system {
                        InitSystem::OpenRC => {
                            let _ = run_shell(&format!("rc-service {} stop", shell_quote(&name))).await;
                            let _ = run_shell(&format!("rc-update del {} default 2>/dev/null", shell_quote(&name))).await;
                        }
                        InitSystem::Procd => {
                            let _ = run_shell(&format!("/etc/init.d/{} stop", shell_quote(&name))).await;
                            let _ = run_shell(&format!("/etc/init.d/{} disable 2>/dev/null", shell_quote(&name))).await;
                        }
                        _ => {}
                    }
                    let _ = fs::remove_file(entry.path());
                }
            }
        }
    }
}

/// Reconcile the full desired set of WireGuard obfuscator services on this node.
pub async fn sync_obfuscators(specs: Vec<proto::ObfuscatorSpec>) {
    let init_system = detect_init_system();
    info!("Synchronizing {} WireGuard obfuscator service(s)...", specs.len());

    let mut desired = HashSet::new();
    for spec in &specs {
        if !spec.enabled {
            continue;
        }
        if let Err(e) = apply_spec(init_system, spec).await {
            error!("Failed to apply obfuscator for {}: {}", spec.interface_name, e);
        } else {
            desired.insert(spec.service_name.clone());
        }
    }

    cleanup_orphans(init_system, &desired).await;
}

#[cfg(test)]
mod tests {
    use super::*;

    fn client_spec() -> proto::ObfuscatorSpec {
        proto::ObfuscatorSpec {
            interface_name: "wg42peer".to_string(),
            service_name: "wg42peer-obfuscator".to_string(),
            role: "phantun_client".to_string(),
            tun_name: "tunptpeer".to_string(),
            tun_local: "192.168.200.1".to_string(),
            tun_peer: "192.168.200.2".to_string(),
            local_port: 30000,
            server_port: 30001,
            wireguard_port: 0,
            remote: "203.0.113.5:30001".to_string(),
            fwmark: "0x42".to_string(),
            extra_args: vec!["--tun-mtu".to_string(), "1400".to_string()],
            enabled: true,
        }
    }

    #[test]
    fn builds_client_args() {
        let args = build_args(&client_spec(), "/usr/bin/phantun_client").unwrap();
        assert_eq!(args[0], "/usr/bin/phantun_client");
        assert!(args.windows(2).any(|w| w == ["--local", "0.0.0.0:30000"]));
        assert!(args.windows(2).any(|w| w == ["--remote", "203.0.113.5:30001"]));
        assert!(args.windows(2).any(|w| w == ["--tun", "tunptpeer"]));
        assert!(args.windows(2).any(|w| w == ["--fwmark", "0x42"]));
        assert!(args.iter().any(|a| a == "--tun-mtu"));
    }

    #[test]
    fn builds_server_args() {
        let mut spec = client_spec();
        spec.role = "phantun_server".to_string();
        spec.local_port = 30001;
        spec.wireguard_port = 25532;
        spec.remote = String::new();

        let args = build_args(&spec, "/usr/bin/phantun_server").unwrap();
        assert!(args.windows(2).any(|w| w == ["--local", "30001"]));
        assert!(args.windows(2).any(|w| w == ["--remote", "127.0.0.1:25532"]));
        assert!(!args.iter().any(|a| a == "--keepalive-time"));
    }

    #[test]
    fn rejects_unsafe_extra_args() {
        let mut spec = client_spec();
        spec.extra_args = vec!["--foo;rm -rf /".to_string()];
        assert!(build_args(&spec, "/usr/bin/phantun_client").is_err());
    }
}
