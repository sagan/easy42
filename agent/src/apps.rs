use sha2::{Digest, Sha256};
use std::collections::{HashMap, HashSet};
use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use tracing::{error, info};

use crate::collector::proto::{AppActionCmd, AppConfigFile, AppSpec, AppStatus, AppType};
use crate::executor::run_shell;

pub const DOCKER_BASE_DIR: &str = "/etc/ezappc";
pub const SYSTEMD_SYSTEM_DIR: &str = "/etc/systemd/system";
pub const OPENRC_INIT_DIR: &str = "/etc/init.d";
pub const OPENRC_CONF_DIR: &str = "/etc/conf.d";
pub const PROCD_INIT_DIR: &str = "/etc/init.d";
pub const APP_DATA_BASE_DIR: &str = "/etc/ezapp-data";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum InitSystem {
    Systemd,
    OpenRC,
    Procd,
}

pub fn detect_init_system() -> InitSystem {
    if Path::new("/run/systemd/system").exists() {
        InitSystem::Systemd
    } else if Path::new("/etc/openwrt_release").exists() || Path::new("/sbin/procd").exists() {
        InitSystem::Procd
    } else if Path::new("/sbin/openrc-run").exists() || Path::new("/run/openrc").exists() {
        InitSystem::OpenRC
    } else {
        InitSystem::Systemd
    }
}

pub struct AppsManager {
    init_system: InitSystem,
}

impl AppsManager {
    pub fn new() -> Self {
        let init_system = detect_init_system();
        info!("AppsManager initialized with init system: {:?}", init_system);
        Self { init_system }
    }

    /// Sync apps state declaratively: install/update desired apps and garbage collect removed apps
    pub async fn sync_apps(&self, desired_apps: Vec<AppSpec>) {
        info!("Synchronizing {} app(s)...", desired_apps.len());

        let mut desired_names = HashSet::new();
        for app in &desired_apps {
            desired_names.insert(app.name.clone());
        }

        // 1. Install or update each desired app
        for app in &desired_apps {
            if let Err(e) = self.apply_app(app).await {
                error!("Failed to apply app '{}': {}", app.name, e);
            }
        }

        // 2. Garbage collect orphaned apps
        self.cleanup_orphans(&desired_names).await;
    }

    /// Calculate current sha256 hashes of deployed app service & env files on this node
    pub fn get_installed_app_hashes(&self) -> HashMap<String, String> {
        let mut hashes = HashMap::new();

        // 1. Docker apps in /etc/ezappc/<name>
        if let Ok(entries) = fs::read_dir(DOCKER_BASE_DIR) {
            for entry in entries.flatten() {
                if let Ok(ft) = entry.file_type() {
                    if ft.is_dir() {
                        let name = entry.file_name().to_string_lossy().to_string();
                        let app_dir = entry.path();
                        let compose_path = app_dir.join("compose.yaml");
                        let env_path = app_dir.join(".env");
                        if compose_path.exists() {
                            let main_content = fs::read_to_string(&compose_path).unwrap_or_default();
                            let env_content = fs::read_to_string(&env_path).unwrap_or_default();
                            let mut files = Vec::new();
                            read_auxiliary_files_recursive(&app_dir, &app_dir, &mut files);
                            let hash = compute_files_hash(&main_content, &env_content, &files);
                            hashes.insert(name, hash);
                        }
                    }
                }
            }
        }

        // 2. Systemd apps in /etc/systemd/system/ezapp-*.service
        if Path::new(SYSTEMD_SYSTEM_DIR).exists() {
            if let Ok(entries) = fs::read_dir(SYSTEMD_SYSTEM_DIR) {
                for entry in entries.flatten() {
                    let file_name = entry.file_name().to_string_lossy().to_string();
                    if file_name.starts_with("ezapp-") && file_name.ends_with(".service") {
                        let name = file_name
                            .trim_start_matches("ezapp-")
                            .trim_end_matches(".service")
                            .to_string();
                        let service_path = entry.path();
                        let env_path = PathBuf::from(SYSTEMD_SYSTEM_DIR).join(format!("ezapp-{}.env", name));
                        let data_dir = PathBuf::from(APP_DATA_BASE_DIR).join(&name);

                        let main_content = fs::read_to_string(&service_path).unwrap_or_default();
                        let env_content = fs::read_to_string(&env_path).unwrap_or_default();
                        let mut files = Vec::new();
                        if data_dir.is_dir() {
                            read_auxiliary_files_recursive(&data_dir, &data_dir, &mut files);
                        }
                        let hash = compute_files_hash(&main_content, &env_content, &files);
                        hashes.insert(name, hash);
                    }
                }
            }
        }

        // 3. OpenRC / Procd in /etc/init.d/ezapp-*
        if Path::new(OPENRC_INIT_DIR).exists() && self.init_system != InitSystem::Systemd {
            if let Ok(entries) = fs::read_dir(OPENRC_INIT_DIR) {
                for entry in entries.flatten() {
                    let file_name = entry.file_name().to_string_lossy().to_string();
                    if file_name.starts_with("ezapp-") {
                        let name = file_name.trim_start_matches("ezapp-").to_string();
                        let init_path = entry.path();
                        let data_dir = PathBuf::from(APP_DATA_BASE_DIR).join(&name);

                        let main_content = fs::read_to_string(&init_path).unwrap_or_default();
                        let env_content = match self.init_system {
                            InitSystem::OpenRC => {
                                let conf_path = PathBuf::from(OPENRC_CONF_DIR).join(format!("ezapp-{}", name));
                                fs::read_to_string(&conf_path).unwrap_or_default()
                            }
                            _ => String::new(),
                        };
                        let mut files = Vec::new();
                        if data_dir.is_dir() {
                            read_auxiliary_files_recursive(&data_dir, &data_dir, &mut files);
                        }
                        let hash = compute_files_hash(&main_content, &env_content, &files);
                        hashes.insert(name, hash);
                    }
                }
            }
        }

        hashes
    }

    async fn apply_app(&self, app: &AppSpec) -> Result<(), String> {
        let app_type = match AppType::try_from(app.app_type) {
            Ok(t) => t,
            Err(_) => AppType::DockerCompose,
        };

        match app_type {
            AppType::DockerCompose => self.apply_docker_app(app).await,
            AppType::Systemd => self.apply_native_app(app).await,
        }
    }

    async fn apply_docker_app(&self, app: &AppSpec) -> Result<(), String> {
        let app_dir = PathBuf::from(DOCKER_BASE_DIR).join(&app.name);
        fs::create_dir_all(&app_dir).map_err(|e| format!("failed to create dir {:?}: {}", app_dir, e))?;

        let compose_file = app_dir.join("compose.yaml");
        let env_file = app_dir.join(".env");

        // Write compose.yaml
        fs::write(&compose_file, &app.main_content)
            .map_err(|e| format!("failed to write compose.yaml: {}", e))?;

        // Write .env with 0600
        fs::write(&env_file, &app.env_content)
            .map_err(|e| format!("failed to write .env: {}", e))?;
        let _ = fs::set_permissions(&env_file, fs::Permissions::from_mode(0o600));

        // Write auxiliary config files
        self.write_config_files(&app_dir, &app.config_files)?;

        // Execute docker compose
        let project_name = format!("ezapp-{}", app.name);
        let compose_cmd = get_docker_compose_cmd();

        if app.enabled {
            info!("Starting Docker app '{}'...", app.name);
            let cmd = format!(
                "cd {} && {} -p {} --env-file .env up -d --remove-orphans",
                shell_quote(app_dir.to_str().unwrap_or("")),
                compose_cmd,
                shell_quote(&project_name)
            );
            let (success, code, stdout, stderr) = run_shell(&cmd).await;
            if !success {
                return Err(format!("docker compose up failed (code {}): {} {}", code, stdout, stderr));
            }
        } else {
            info!("Stopping disabled Docker app '{}'...", app.name);
            let cmd = format!(
                "cd {} && {} -p {} stop",
                shell_quote(app_dir.to_str().unwrap_or("")),
                compose_cmd,
                shell_quote(&project_name)
            );
            let _ = run_shell(&cmd).await;
        }

        Ok(())
    }

    async fn apply_native_app(&self, app: &AppSpec) -> Result<(), String> {
        // Write auxiliary files to /etc/ezapp-data/<name>/
        let app_data_dir = PathBuf::from(APP_DATA_BASE_DIR).join(&app.name);
        if !app.config_files.is_empty() {
            fs::create_dir_all(&app_data_dir).map_err(|e| format!("failed to create data dir: {}", e))?;
            self.write_config_files(&app_data_dir, &app.config_files)?;
        }

        match self.init_system {
            InitSystem::Systemd => self.apply_systemd_app(app).await,
            InitSystem::OpenRC => self.apply_openrc_app(app).await,
            InitSystem::Procd => self.apply_procd_app(app).await,
        }
    }

    async fn apply_systemd_app(&self, app: &AppSpec) -> Result<(), String> {
        let service_name = format!("ezapp-{}", app.name);
        let service_path = PathBuf::from(SYSTEMD_SYSTEM_DIR).join(format!("{}.service", service_name));
        let env_path = PathBuf::from(SYSTEMD_SYSTEM_DIR).join(format!("{}.env", service_name));

        let main_content = ensure_systemd_env_file(&app.name, &app.main_content);

        fs::write(&service_path, &main_content)
            .map_err(|e| format!("failed to write systemd service unit: {}", e))?;
        fs::write(&env_path, &app.env_content)
            .map_err(|e| format!("failed to write systemd env file: {}", e))?;
        let _ = fs::set_permissions(&env_path, fs::Permissions::from_mode(0o600));

        let _ = run_shell("systemctl daemon-reload").await;

        if app.enabled {
            info!("Starting systemd service '{}'...", service_name);
            let cmd = format!("systemctl enable --now {}", service_name);
            let (success, code, stdout, stderr) = run_shell(&cmd).await;
            if !success {
                return Err(format!("systemctl enable --now failed (code {}): {} {}", code, stdout, stderr));
            }
            let _ = run_shell(&format!("systemctl restart {}", service_name)).await;
        } else {
            info!("Stopping systemd service '{}'...", service_name);
            let _ = run_shell(&format!("systemctl stop {}", service_name)).await;
        }

        Ok(())
    }

    async fn apply_openrc_app(&self, app: &AppSpec) -> Result<(), String> {
        let service_name = format!("ezapp-{}", app.name);
        let init_path = PathBuf::from(OPENRC_INIT_DIR).join(&service_name);
        let conf_path = PathBuf::from(OPENRC_CONF_DIR).join(&service_name);

        fs::write(&init_path, &app.main_content)
            .map_err(|e| format!("failed to write OpenRC init script: {}", e))?;
        let _ = fs::set_permissions(&init_path, fs::Permissions::from_mode(0o755));

        if !app.env_content.is_empty() {
            fs::write(&conf_path, &app.env_content)
                .map_err(|e| format!("failed to write OpenRC conf.d file: {}", e))?;
            let _ = fs::set_permissions(&conf_path, fs::Permissions::from_mode(0o600));
        }

        let _ = run_shell(&format!("rc-update add {} default 2>/dev/null", service_name)).await;

        if app.enabled {
            info!("Restarting OpenRC service '{}'...", service_name);
            let _ = run_shell(&format!("rc-service {} restart", service_name)).await;
        } else {
            info!("Stopping OpenRC service '{}'...", service_name);
            let _ = run_shell(&format!("rc-service {} stop", service_name)).await;
        }

        Ok(())
    }

    async fn apply_procd_app(&self, app: &AppSpec) -> Result<(), String> {
        let service_name = format!("ezapp-{}", app.name);
        let init_path = PathBuf::from(PROCD_INIT_DIR).join(&service_name);

        fs::write(&init_path, &app.main_content)
            .map_err(|e| format!("failed to write Procd init script: {}", e))?;
        let _ = fs::set_permissions(&init_path, fs::Permissions::from_mode(0o755));

        let _ = run_shell(&format!("/etc/init.d/{} enable 2>/dev/null", service_name)).await;

        if app.enabled {
            info!("Restarting Procd service '{}'...", service_name);
            let _ = run_shell(&format!("/etc/init.d/{} restart", service_name)).await;
        } else {
            info!("Stopping Procd service '{}'...", service_name);
            let _ = run_shell(&format!("/etc/init.d/{} stop", service_name)).await;
        }

        Ok(())
    }

    fn write_config_files(&self, base_dir: &Path, files: &[AppConfigFile]) -> Result<(), String> {
        for file in files {
            let rel_path = file.path.trim();
            if rel_path.is_empty() || rel_path.contains("..") || rel_path.starts_with('/') {
                continue;
            }
            let target_path = base_dir.join(rel_path);
            if let Some(parent) = target_path.parent() {
                let _ = fs::create_dir_all(parent);
            }
            fs::write(&target_path, &file.content)
                .map_err(|e| format!("failed to write auxiliary file {:?}: {}", target_path, e))?;
            if file.file_mode != 0 {
                let _ = fs::set_permissions(&target_path, fs::Permissions::from_mode(file.file_mode));
            }
        }
        Ok(())
    }

    /// Garbage collect orphaned apps no longer in the desired set
    async fn cleanup_orphans(&self, desired_names: &HashSet<String>) {
        // 1. Docker apps in /etc/ezappc/
        if let Ok(entries) = fs::read_dir(DOCKER_BASE_DIR) {
            for entry in entries.flatten() {
                if let Ok(file_type) = entry.file_type() {
                    if file_type.is_dir() {
                        let name = entry.file_name().to_string_lossy().to_string();
                        if !desired_names.contains(&name) {
                            info!("Removing orphaned Docker app '{}'...", name);
                            let project_name = format!("ezapp-{}", name);
                            let compose_cmd = get_docker_compose_cmd();
                            let cmd = format!(
                                "cd {} && {} -p {} down --volumes 2>/dev/null || true",
                                shell_quote(entry.path().to_str().unwrap_or("")),
                                compose_cmd,
                                shell_quote(&project_name)
                            );
                            let _ = run_shell(&cmd).await;
                            let _ = fs::remove_dir_all(entry.path());
                        }
                    }
                }
            }
        }

        // 2. Systemd apps in /etc/systemd/system/ezapp-*.service
        if Path::new(SYSTEMD_SYSTEM_DIR).exists() {
            if let Ok(entries) = fs::read_dir(SYSTEMD_SYSTEM_DIR) {
                for entry in entries.flatten() {
                    let file_name = entry.file_name().to_string_lossy().to_string();
                    if file_name.starts_with("ezapp-") && file_name.ends_with(".service") {
                        let name = file_name.trim_start_matches("ezapp-").trim_end_matches(".service");
                        if !desired_names.contains(name) {
                            info!("Removing orphaned systemd app '{}'...", name);
                            let service_name = format!("ezapp-{}", name);
                            let _ = run_shell(&format!("systemctl stop {} 2>/dev/null || true", service_name)).await;
                            let _ = run_shell(&format!("systemctl disable {} 2>/dev/null || true", service_name)).await;
                            let _ = fs::remove_file(entry.path());
                            let _ = fs::remove_file(PathBuf::from(SYSTEMD_SYSTEM_DIR).join(format!("{}.env", service_name)));
                            let _ = fs::remove_dir_all(PathBuf::from(APP_DATA_BASE_DIR).join(name));
                            let _ = run_shell("systemctl daemon-reload").await;
                        }
                    }
                }
            }
        }

        // 3. OpenRC / Procd apps in /etc/init.d/ezapp-*
        if Path::new(OPENRC_INIT_DIR).exists() {
            if let Ok(entries) = fs::read_dir(OPENRC_INIT_DIR) {
                for entry in entries.flatten() {
                    let file_name = entry.file_name().to_string_lossy().to_string();
                    if file_name.starts_with("ezapp-") {
                        let name = file_name.trim_start_matches("ezapp-");
                        if !desired_names.contains(name) {
                            info!("Removing orphaned init app '{}'...", name);
                            let service_name = format!("ezapp-{}", name);
                            match self.init_system {
                                InitSystem::OpenRC => {
                                    let _ = run_shell(&format!("rc-service {} stop 2>/dev/null || true", service_name)).await;
                                    let _ = run_shell(&format!("rc-update del {} default 2>/dev/null || true", service_name)).await;
                                    let _ = fs::remove_file(entry.path());
                                    let _ = fs::remove_file(PathBuf::from(OPENRC_CONF_DIR).join(&service_name));
                                }
                                InitSystem::Procd => {
                                    let _ = run_shell(&format!("/etc/init.d/{} stop 2>/dev/null || true", service_name)).await;
                                    let _ = run_shell(&format!("/etc/init.d/{} disable 2>/dev/null || true", service_name)).await;
                                    let _ = fs::remove_file(entry.path());
                                }
                                _ => {}
                            }
                            let _ = fs::remove_dir_all(PathBuf::from(APP_DATA_BASE_DIR).join(name));
                        }
                    }
                }
            }
        }
    }

    /// Query live status of all installed apps on the host
    pub async fn collect_app_statuses(&self) -> Vec<AppStatus> {
        let mut statuses = Vec::new();
        let mut seen = HashSet::new();

        // 1. Check Docker apps in /etc/ezappc/
        if let Ok(entries) = fs::read_dir(DOCKER_BASE_DIR) {
            for entry in entries.flatten() {
                if let Ok(ft) = entry.file_type() {
                    if ft.is_dir() {
                        let name = entry.file_name().to_string_lossy().to_string();
                        if seen.insert(name.clone()) {
                            let (state, text, err) = self.probe_docker_status(&name).await;
                            statuses.push(AppStatus {
                                app_id: String::new(),
                                name,
                                app_type: AppType::DockerCompose as i32,
                                state,
                                status_text: text,
                                last_updated: chrono_now(),
                                error_message: err,
                            });
                        }
                    }
                }
            }
        }

        // 2. Check Native Systemd apps
        if self.init_system == InitSystem::Systemd && Path::new(SYSTEMD_SYSTEM_DIR).exists() {
            if let Ok(entries) = fs::read_dir(SYSTEMD_SYSTEM_DIR) {
                for entry in entries.flatten() {
                    let file_name = entry.file_name().to_string_lossy().to_string();
                    if file_name.starts_with("ezapp-") && file_name.ends_with(".service") {
                        let name = file_name.trim_start_matches("ezapp-").trim_end_matches(".service").to_string();
                        if seen.insert(name.clone()) {
                            let (state, text, err) = self.probe_systemd_status(&name).await;
                            statuses.push(AppStatus {
                                app_id: String::new(),
                                name,
                                app_type: AppType::Systemd as i32,
                                state,
                                status_text: text,
                                last_updated: chrono_now(),
                                error_message: err,
                            });
                        }
                    }
                }
            }
        }

        // 3. Check Native OpenRC / Procd apps
        if self.init_system != InitSystem::Systemd && Path::new(OPENRC_INIT_DIR).exists() {
            if let Ok(entries) = fs::read_dir(OPENRC_INIT_DIR) {
                for entry in entries.flatten() {
                    let file_name = entry.file_name().to_string_lossy().to_string();
                    if file_name.starts_with("ezapp-") {
                        let name = file_name.trim_start_matches("ezapp-").to_string();
                        if seen.insert(name.clone()) {
                            let (state, text, err) = self.probe_init_status(&name).await;
                            statuses.push(AppStatus {
                                app_id: String::new(),
                                name,
                                app_type: AppType::Systemd as i32,
                                state,
                                status_text: text,
                                last_updated: chrono_now(),
                                error_message: err,
                            });
                        }
                    }
                }
            }
        }

        statuses
    }

    async fn probe_docker_status(&self, name: &str) -> (String, String, String) {
        let project_name = format!("ezapp-{}", name);
        let compose_cmd = get_docker_compose_cmd();
        let app_dir = PathBuf::from(DOCKER_BASE_DIR).join(name);

        let cmd = format!(
            "cd {} && {} -p {} ps --format '{{{{.State}}}} {{{{.Status}}}}' 2>/dev/null",
            shell_quote(app_dir.to_str().unwrap_or("")),
            compose_cmd,
            shell_quote(&project_name)
        );

        let (success, _, stdout, stderr) = run_shell(&cmd).await;
        if !success || stdout.trim().is_empty() {
            return ("stopped".to_string(), "stopped or not started".to_string(), stderr);
        }

        let first_line = stdout.lines().next().unwrap_or("").trim();
        if first_line.to_lowercase().starts_with("running") {
            ("running".to_string(), first_line.to_string(), String::new())
        } else {
            ("stopped".to_string(), first_line.to_string(), String::new())
        }
    }

    async fn probe_systemd_status(&self, name: &str) -> (String, String, String) {
        let service = format!("ezapp-{}", name);
        let cmd = format!("systemctl is-active {}", service);
        let (_success, _, stdout, stderr) = run_shell(&cmd).await;
        let active_state = stdout.trim();

        if active_state == "active" {
            ("running".to_string(), "active (running)".to_string(), String::new())
        } else if active_state == "failed" {
            ("failed".to_string(), "failed".to_string(), stderr)
        } else {
            ("stopped".to_string(), active_state.to_string(), String::new())
        }
    }

    async fn probe_init_status(&self, name: &str) -> (String, String, String) {
        let service = format!("ezapp-{}", name);
        let cmd = match self.init_system {
            InitSystem::OpenRC => format!("rc-service {} status 2>&1", service),
            InitSystem::Procd => format!("/etc/init.d/{} status 2>&1", service),
            _ => format!("service {} status 2>&1", service),
        };

        let (success, _, stdout, stderr) = run_shell(&cmd).await;
        let out = stdout.trim();
        let lower = out.to_lowercase();

        if lower.contains("started") || lower.contains("running") {
            ("running".to_string(), out.to_string(), String::new())
        } else if lower.contains("stopped") {
            ("stopped".to_string(), out.to_string(), String::new())
        } else if success {
            ("running".to_string(), out.to_string(), String::new())
        } else {
            ("stopped".to_string(), out.to_string(), stderr)
        }
    }

    /// Execute app action (start, stop, restart, logs)
    pub async fn handle_action(&self, req: AppActionCmd) -> (bool, i32, String, String) {
        let app_type = match AppType::try_from(req.app_type) {
            Ok(t) => t,
            Err(_) => AppType::DockerCompose,
        };

        let action = req.action.to_lowercase();
        let lines = if req.log_lines > 0 { req.log_lines } else { 100 };

        match app_type {
            AppType::DockerCompose => {
                let app_dir = PathBuf::from(DOCKER_BASE_DIR).join(&req.app_name);
                let project_name = format!("ezapp-{}", req.app_name);
                let compose_cmd = get_docker_compose_cmd();

                let cmd = match action.as_str() {
                    "start" => format!("cd {} && {} -p {} start", shell_quote(app_dir.to_str().unwrap_or("")), compose_cmd, shell_quote(&project_name)),
                    "stop" => format!("cd {} && {} -p {} stop", shell_quote(app_dir.to_str().unwrap_or("")), compose_cmd, shell_quote(&project_name)),
                    "restart" => format!("cd {} && {} -p {} restart", shell_quote(app_dir.to_str().unwrap_or("")), compose_cmd, shell_quote(&project_name)),
                    "logs" => format!("cd {} && {} -p {} logs --tail {}", shell_quote(app_dir.to_str().unwrap_or("")), compose_cmd, shell_quote(&project_name), lines),
                    other => return (false, -1, String::new(), format!("unknown action '{}'", other)),
                };
                run_shell(&cmd).await
            }
            AppType::Systemd => {
                let service = format!("ezapp-{}", req.app_name);
                match self.init_system {
                    InitSystem::Systemd => {
                        let cmd = match action.as_str() {
                            "start" => format!("systemctl start {}", service),
                            "stop" => format!("systemctl stop {}", service),
                            "restart" => format!("systemctl restart {}", service),
                            "logs" => format!("journalctl -u {} -n {} --no-pager", service, lines),
                            other => return (false, -1, String::new(), format!("unknown action '{}'", other)),
                        };
                        run_shell(&cmd).await
                    }
                    InitSystem::OpenRC => {
                        let cmd = match action.as_str() {
                            "start" => format!("rc-service {} start", service),
                            "stop" => format!("rc-service {} stop", service),
                            "restart" => format!("rc-service {} restart", service),
                            "logs" => format!("rc-service {} status", service),
                            other => return (false, -1, String::new(), format!("unknown action '{}'", other)),
                        };
                        run_shell(&cmd).await
                    }
                    InitSystem::Procd => {
                        let cmd = match action.as_str() {
                            "start" => format!("/etc/init.d/{} start", service),
                            "stop" => format!("/etc/init.d/{} stop", service),
                            "restart" => format!("/etc/init.d/{} restart", service),
                            "logs" => format!("/etc/init.d/{} status", service),
                            other => return (false, -1, String::new(), format!("unknown action '{}'", other)),
                        };
                        run_shell(&cmd).await
                    }
                }
            }
        }
    }
}

fn get_docker_compose_cmd() -> &'static str {
    // Default to modern "docker compose", fallback will be handled by shell if alias exists
    "docker compose"
}

fn shell_quote(s: &str) -> String {
    format!("'{}'", s.replace('\'', "'\\''"))
}

fn chrono_now() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

pub fn ensure_systemd_env_file(app_name: &str, content: &str) -> String {
    let app_name = app_name.trim();
    let app_name = if app_name.is_empty() { "sample" } else { app_name };
    let expected_path = format!("/etc/systemd/system/ezapp-{}.env", app_name);

    if content.contains(&expected_path) {
        return content.to_string();
    }

    let sample_path = "/etc/systemd/system/ezapp-sample.env";
    if content.contains(sample_path) {
        return content.replacen(sample_path, &expected_path, 1);
    }

    let expected_directive = format!("EnvironmentFile={}", expected_path);

    let lines: Vec<&str> = content.lines().collect();
    let mut service_idx = None;
    for (i, line) in lines.iter().enumerate() {
        let trimmed = line.trim();
        if trimmed.starts_with("[Service]") {
            service_idx = Some(i);
            break;
        }
    }

    if let Some(idx) = service_idx {
        let mut new_lines = Vec::with_capacity(lines.len() + 1);
        new_lines.extend_from_slice(&lines[..=idx]);
        new_lines.push(&expected_directive);
        new_lines.extend_from_slice(&lines[idx + 1..]);
        let mut res = new_lines.join("\n");
        if content.ends_with('\n') {
            res.push('\n');
        }
        res
    } else {
        let trimmed = content.trim_end();
        if trimmed.is_empty() {
            format!("[Service]\n{}\n", expected_directive)
        } else {
            format!("{}\n\n[Service]\n{}\n", trimmed, expected_directive)
        }
    }
}

fn read_auxiliary_files_recursive(root_dir: &Path, current_dir: &Path, out: &mut Vec<(String, String)>) {
    if let Ok(entries) = fs::read_dir(current_dir) {
        for entry in entries.flatten() {
            let path = entry.path();
            if let Ok(ft) = entry.file_type() {
                if ft.is_dir() {
                    read_auxiliary_files_recursive(root_dir, &path, out);
                } else if ft.is_file() {
                    if let Ok(rel) = path.strip_prefix(root_dir) {
                        let rel_str = rel.to_string_lossy().to_string();
                        if rel_str == "compose.yaml" || rel_str == ".env" {
                            continue;
                        }
                        if let Ok(content) = fs::read_to_string(&path) {
                            out.push((rel_str, content));
                        }
                    }
                }
            }
        }
    }
}

/// Compute a deterministic SHA-256 hash across main service content, env content, and auxiliary files
pub fn compute_files_hash(main: &str, env: &str, files: &[(String, String)]) -> String {
    let mut hasher = Sha256::new();
    let norm_main = main.replace("\r\n", "\n");
    let norm_main = norm_main.trim_end();
    let norm_env = env.replace("\r\n", "\n");
    let norm_env = norm_env.trim_end();

    hasher.update(format!("main:\n{}\n", norm_main).as_bytes());
    hasher.update(format!("env:\n{}\n", norm_env).as_bytes());

    let mut sorted_files: Vec<(String, String)> = files.to_vec();
    sorted_files.sort_by(|a, b| a.0.cmp(&b.0));
    for (rel_path, content) in sorted_files {
        let norm_f = content.replace("\r\n", "\n");
        let norm_f = norm_f.trim_end();
        hasher.update(format!("file:{}:\n{}\n", rel_path, norm_f).as_bytes());
    }

    format!("{:x}", hasher.finalize())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_compute_files_hash() {
        let main = "[Unit]\nDescription=rclone\n";
        let env = "RCLONE_PASS=123456\r\n";
        let files = vec![("conf/rclone.conf".to_string(), "[drive]\ntype=drive\n".to_string())];
        let h1 = compute_files_hash(main, env, &files);
        assert_eq!(h1.len(), 64);

        // Whitespace and newline difference normalization
        let main2 = "[Unit]\nDescription=rclone   \r\n";
        let env2 = "RCLONE_PASS=123456\n\n";
        let h2 = compute_files_hash(main2, env2, &files);
        assert_eq!(h1, h2);

        // Content change changes hash
        let main3 = "[Unit]\nDescription=rclone modified\n";
        let h3 = compute_files_hash(main3, env, &files);
        assert_ne!(h1, h3);
    }

    #[test]
    fn test_ensure_systemd_env_file_missing() {
        let raw = "[Unit]\nDescription=rclone service\n\n[Service]\nType=simple\nExecStart=rclone\n";
        let res = ensure_systemd_env_file("rclone", raw);
        assert!(res.contains("EnvironmentFile=/etc/systemd/system/ezapp-rclone.env"));
        assert!(res.contains("[Service]\nEnvironmentFile=/etc/systemd/system/ezapp-rclone.env\nType=simple"));
    }

    #[test]
    fn test_ensure_systemd_env_file_sample_replaced() {
        let raw = "[Unit]\nDescription=sample\n\n[Service]\nEnvironmentFile=/etc/systemd/system/ezapp-sample.env\nExecStart=test\n";
        let res = ensure_systemd_env_file("myapp", raw);
        assert!(res.contains("EnvironmentFile=/etc/systemd/system/ezapp-myapp.env"));
        assert!(!res.contains("ezapp-sample.env"));
    }

    #[test]
    fn test_ensure_systemd_env_file_already_present() {
        let raw = "[Unit]\nDescription=sample\n\n[Service]\nEnvironmentFile=/etc/systemd/system/ezapp-myapp.env\nExecStart=test\n";
        let res = ensure_systemd_env_file("myapp", raw);
        assert_eq!(res, raw);
    }
}


