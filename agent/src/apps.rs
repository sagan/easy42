use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::{HashMap, HashSet};
use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use tracing::{error, info};

use std::sync::Arc;
use tokio::sync::Mutex;

use crate::collector::proto::{AppActionCmd, AppConfigFile, AppSpec, AppStatus, AppType};
use crate::container::compose_command;
use crate::executor::{run_shell, run_shell_with_timeout};

pub const DOCKER_BASE_DIR: &str = "/etc/ezappc";
pub const SYSTEMD_SYSTEM_DIR: &str = "/etc/systemd/system";
pub const OPENRC_INIT_DIR: &str = "/etc/init.d";
pub const OPENRC_CONF_DIR: &str = "/etc/conf.d";
pub const PROCD_INIT_DIR: &str = "/etc/init.d";
pub const APP_DATA_BASE_DIR: &str = "/etc/ezapp-data";
/// Hidden manifest listing the files Easy42 generated inside an app directory.
/// It lets the agent distinguish managed files from data created by the app
/// (for example bind-mounted volumes) during updates and garbage collection.
pub const MANAGED_MANIFEST_FILE: &str = ".easy42-managed.json";

#[derive(Serialize, Deserialize, Default)]
struct ManagedFilesManifest {
    #[serde(default)]
    files: Vec<String>,
}

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

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct AppInstalledFile {
    pub path: String,
    pub content: String,
    pub hash: String,
}

pub fn hash_single_content(content: &str) -> String {
    let mut hasher = Sha256::new();
    let normalized = content.replace("\r\n", "\n");
    let normalized_trimmed = normalized.trim_end();
    hasher.update(normalized_trimmed.as_bytes());
    format!("{:x}", hasher.finalize())
}

pub struct AppsManager {
    init_system: InitSystem,
    starting_apps: Arc<Mutex<HashMap<String, i64>>>,
    app_errors: Arc<Mutex<HashMap<String, String>>>,
    installed_cron_apps: Arc<Mutex<Vec<AppSpec>>>,
}

impl AppsManager {
    pub fn new() -> Self {
        let init_system = detect_init_system();
        info!("AppsManager initialized with init system: {:?}", init_system);
        Self {
            init_system,
            starting_apps: Arc::new(Mutex::new(HashMap::new())),
            app_errors: Arc::new(Mutex::new(HashMap::new())),
            installed_cron_apps: Arc::new(Mutex::new(Vec::new())),
        }
    }

    /// Sync apps state declaratively: install/update desired apps and garbage collect removed apps
    pub async fn sync_apps(&self, desired_apps: Vec<AppSpec>) {
        info!("Synchronizing {} app(s)...", desired_apps.len());

        let mut desired_names = HashSet::new();
        let mut cron_apps = Vec::new();
        for app in &desired_apps {
            desired_names.insert(app.name.clone());
            if app.app_type == AppType::Cron as i32 {
                cron_apps.push(app);
            }
        }

        // 1. Sync crontab managed by Easy42
        if let Err(e) = crate::crontab::sync_crontab_apps(&cron_apps).await {
            error!("Failed to sync crontab apps: {}", e);
        }
        {
            let mut lock = self.installed_cron_apps.lock().await;
            *lock = desired_apps
                .iter()
                .filter(|a| a.app_type == AppType::Cron as i32)
                .cloned()
                .collect();
        }

        // 2. Install or update non-cron desired apps
        for app in &desired_apps {
            if app.app_type != AppType::Cron as i32 {
                if let Err(e) = self.apply_app(app).await {
                    error!("Failed to apply app '{}': {}", app.name, e);
                }
            }
        }

        // 3. Garbage collect orphaned apps
        self.cleanup_orphans(&desired_names).await;
    }

    /// Calculate current sha256 hashes of deployed apps and collect deployed files with contents
    pub fn get_installed_app_info(&self) -> (HashMap<String, String>, HashMap<String, AppInstalledFile>) {
        let mut hashes = HashMap::new();
        let mut files_map = HashMap::new();

        // 1. Container (compose) apps in /etc/ezappc/<name>
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
                            let files = read_managed_files(&app_dir);
                            let hash = compute_files_hash(&main_content, &env_content, &files);
                            hashes.insert(name.clone(), hash);

                            let comp_path_str = compose_path.to_string_lossy().to_string();
                            files_map.insert(
                                comp_path_str.clone(),
                                AppInstalledFile {
                                    path: comp_path_str,
                                    hash: hash_single_content(&main_content),
                                    content: main_content,
                                },
                            );
                            if env_path.exists() {
                                let env_path_str = env_path.to_string_lossy().to_string();
                                files_map.insert(
                                    env_path_str.clone(),
                                    AppInstalledFile {
                                        path: env_path_str,
                                        hash: hash_single_content(&env_content),
                                        content: env_content,
                                    },
                                );
                            }
                            for (rel_path, content) in files {
                                let abs_path = app_dir.join(&rel_path).to_string_lossy().to_string();
                                files_map.insert(
                                    abs_path.clone(),
                                    AppInstalledFile {
                                        path: abs_path,
                                        hash: hash_single_content(&content),
                                        content,
                                    },
                                );
                            }
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
                        let files = read_managed_files(&data_dir);
                        let hash = compute_files_hash(&main_content, &env_content, &files);
                        hashes.insert(name.clone(), hash);

                        let srv_path_str = service_path.to_string_lossy().to_string();
                        files_map.insert(
                            srv_path_str.clone(),
                            AppInstalledFile {
                                path: srv_path_str,
                                hash: hash_single_content(&main_content),
                                content: main_content,
                            },
                        );
                        if env_path.exists() {
                            let env_path_str = env_path.to_string_lossy().to_string();
                            files_map.insert(
                                env_path_str.clone(),
                                AppInstalledFile {
                                    path: env_path_str,
                                    hash: hash_single_content(&env_content),
                                    content: env_content,
                                },
                            );
                        }
                        for (rel_path, content) in files {
                            let abs_path = data_dir.join(&rel_path).to_string_lossy().to_string();
                            files_map.insert(
                                abs_path.clone(),
                                AppInstalledFile {
                                    path: abs_path,
                                    hash: hash_single_content(&content),
                                    content,
                                },
                            );
                        }
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
                        let (env_content, env_exists, env_path_str) = match self.init_system {
                            InitSystem::OpenRC => {
                                let conf_path = PathBuf::from(OPENRC_CONF_DIR).join(format!("ezapp-{}", name));
                                let exists = conf_path.exists();
                                let content = fs::read_to_string(&conf_path).unwrap_or_default();
                                (content, exists, conf_path.to_string_lossy().to_string())
                            }
                            _ => (String::new(), false, String::new()),
                        };
                        let files = read_managed_files(&data_dir);
                        let hash = compute_files_hash(&main_content, &env_content, &files);
                        hashes.insert(name.clone(), hash);

                        let init_path_str = init_path.to_string_lossy().to_string();
                        files_map.insert(
                            init_path_str.clone(),
                            AppInstalledFile {
                                path: init_path_str,
                                hash: hash_single_content(&main_content),
                                content: main_content,
                            },
                        );
                        if env_exists {
                            files_map.insert(
                                env_path_str.clone(),
                                AppInstalledFile {
                                    path: env_path_str,
                                    hash: hash_single_content(&env_content),
                                    content: env_content,
                                },
                            );
                        }
                        for (rel_path, content) in files {
                            let abs_path = data_dir.join(&rel_path).to_string_lossy().to_string();
                            files_map.insert(
                                abs_path.clone(),
                                AppInstalledFile {
                                    path: abs_path,
                                    hash: hash_single_content(&content),
                                    content,
                                },
                            );
                        }
                    }
                }
            }
        }

        (hashes, files_map)
    }

    /// Calculate current sha256 hashes of deployed app service & env files on this node
    pub fn get_installed_app_hashes(&self) -> HashMap<String, String> {
        self.get_installed_app_info().0
    }

    async fn apply_app(&self, app: &AppSpec) -> Result<(), String> {
        let app_type = match AppType::try_from(app.app_type) {
            Ok(t) => t,
            Err(_) => AppType::DockerCompose,
        };

        match app_type {
            AppType::DockerCompose => self.apply_docker_app(app).await,
            AppType::Systemd => self.apply_native_app(app).await,
            AppType::Cron => Ok(()),
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

        // Sync auxiliary config files (write desired ones, remove orphaned ones)
        self.sync_config_files(&app_dir, &app.config_files, &["compose.yaml", ".env"])?;

        // Execute the compose command using whichever container runtime is installed
        let project_name = format!("ezapp-{}", app.name);
        let compose_cmd = compose_command();

        if app.enabled {
            info!("Starting container app '{}' asynchronously...", app.name);
            let app_name = app.name.clone();
            let starting_map = self.starting_apps.clone();
            let errors_map = self.app_errors.clone();

            // Clear any previous error and check if already in-flight starting
            {
                let mut errors = errors_map.lock().await;
                errors.remove(&app_name);
                let mut starting = starting_map.lock().await;
                if starting.contains_key(&app_name) {
                    info!("Container app '{}' is already in-flight starting/pulling; skipping duplicate spawn", app_name);
                    return Ok(());
                }
                starting.insert(app_name.clone(), chrono_now());
            }

            tokio::spawn(async move {
                let cmd = format!(
                    "cd {} && {} -p {} --env-file .env up -d --remove-orphans",
                    shell_quote(app_dir.to_str().unwrap_or("")),
                    compose_cmd,
                    shell_quote(&project_name)
                );
                // Allow up to 15 minutes for image pulls and container initialization
                let (success, code, stdout, stderr) =
                    run_shell_with_timeout(&cmd, std::time::Duration::from_secs(900)).await;

                {
                    let mut starting = starting_map.lock().await;
                    starting.remove(&app_name);
                }

                if !success {
                    let err_msg = format!("{} up failed (code {}): {} {}", compose_cmd, code, stdout, stderr);
                    error!("Container app '{}' background startup error: {}", app_name, err_msg);
                    let mut errors = errors_map.lock().await;
                    errors.insert(app_name, err_msg);
                } else {
                    info!("Container app '{}' background startup succeeded", app_name);
                }
            });
        } else {
            info!("Stopping disabled container app '{}'...", app.name);
            {
                let mut starting = self.starting_apps.lock().await;
                starting.remove(&app.name);
            }
            // `stop` leaves the containers around in an exited state, which is
            // not enough to consider the app disabled. Tear the compose project
            // down instead so nothing keeps running.
            let cmd = format!(
                "cd {} && {} -p {} down --remove-orphans",
                shell_quote(app_dir.to_str().unwrap_or("")),
                compose_cmd,
                shell_quote(&project_name)
            );
            let (success, code, stdout, stderr) = run_shell(&cmd).await;
            if !success {
                error!(
                    "{} down failed for disabled app '{}' (code {}): {} {}; falling back to kill",
                    compose_cmd, app.name, code, stdout, stderr
                );
                let kill_cmd = format!(
                    "cd {} && {} -p {} kill",
                    shell_quote(app_dir.to_str().unwrap_or("")),
                    compose_cmd,
                    shell_quote(&project_name)
                );
                let _ = run_shell(&kill_cmd).await;
            }
        }

        Ok(())
    }

    async fn apply_native_app(&self, app: &AppSpec) -> Result<(), String> {
        // Write auxiliary files to /etc/ezapp-data/<name>/
        let app_data_dir = PathBuf::from(APP_DATA_BASE_DIR).join(&app.name);
        self.sync_config_files(&app_data_dir, &app.config_files, &[])?;

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

    fn sync_config_files(
        &self,
        base_dir: &Path,
        files: &[AppConfigFile],
        excluded_rel_paths: &[&str],
    ) -> Result<(), String> {
        let mut desired_rel_paths = HashSet::new();
        for file in files {
            let rel_path = file.path.trim();
            if is_safe_rel_path(rel_path) {
                desired_rel_paths.insert(rel_path.to_string());
            }
        }

        // Only delete files that were previously written by Easy42. Scanning the
        // directory instead would treat app-generated data (e.g. bind-mounted
        // volumes) as orphaned config and destroy it.
        for rel_path in read_managed_manifest(base_dir) {
            if excluded_rel_paths.contains(&rel_path.as_str()) {
                continue;
            }
            if !desired_rel_paths.contains(&rel_path) {
                let target_path = base_dir.join(&rel_path);
                info!("Removing orphaned app config file: {:?}", target_path);
                let _ = fs::remove_file(&target_path);
                prune_empty_parent_dirs(base_dir, &target_path);
            }
        }

        let manifest_rel_paths: HashSet<String> = desired_rel_paths
            .iter()
            .filter(|rel_path| !excluded_rel_paths.contains(&rel_path.as_str()))
            .cloned()
            .collect();

        if !desired_rel_paths.is_empty() {
            fs::create_dir_all(base_dir)
                .map_err(|e| format!("failed to create config dir {:?}: {}", base_dir, e))?;
            self.write_config_files(base_dir, files)?;
            write_managed_manifest(base_dir, &manifest_rel_paths)?;
        } else {
            write_managed_manifest(base_dir, &HashSet::new())?;
            // Remove the directory only when no app-generated data remains.
            let _ = fs::remove_dir(base_dir);
        }

        Ok(())
    }

    fn write_config_files(&self, base_dir: &Path, files: &[AppConfigFile]) -> Result<(), String> {
        for file in files {
            let rel_path = file.path.trim();
            if !is_safe_rel_path(rel_path) {
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
        // 1. Container (compose) apps in /etc/ezappc/
        if let Ok(entries) = fs::read_dir(DOCKER_BASE_DIR) {
            for entry in entries.flatten() {
                if let Ok(file_type) = entry.file_type() {
                    if file_type.is_dir() {
                        let name = entry.file_name().to_string_lossy().to_string();
                        if !desired_names.contains(&name) {
                            let app_dir = entry.path();
                            // A directory that has neither an Easy42 compose file nor
                            // a managed-file manifest is just leftover user data (for
                            // example a mounted volume). Leave it untouched.
                            if !app_dir.join("compose.yaml").exists()
                                && !app_dir.join(MANAGED_MANIFEST_FILE).exists()
                            {
                                continue;
                            }
                            info!("Removing orphaned container app '{}'...", name);
                            {
                                let mut starting = self.starting_apps.lock().await;
                                starting.remove(&name);
                                let mut errors = self.app_errors.lock().await;
                                errors.remove(&name);
                            }
                            let project_name = format!("ezapp-{}", name);
                            let compose_cmd = compose_command();
                            let cmd = format!(
                                "cd {} && {} -p {} down --volumes --remove-orphans 2>/dev/null || true",
                                shell_quote(app_dir.to_str().unwrap_or("")),
                                compose_cmd,
                                shell_quote(&project_name)
                            );
                            let _ = run_shell(&cmd).await;
                            remove_managed_app_files(&app_dir, &["compose.yaml", ".env"]);
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
                            remove_managed_app_files(&PathBuf::from(APP_DATA_BASE_DIR).join(name), &[]);
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
                            remove_managed_app_files(&PathBuf::from(APP_DATA_BASE_DIR).join(name), &[]);
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

        // 1. Check container (compose) apps in /etc/ezappc/
        if let Ok(entries) = fs::read_dir(DOCKER_BASE_DIR) {
            for entry in entries.flatten() {
                if let Ok(ft) = entry.file_type() {
                    if ft.is_dir() {
                        let app_dir = entry.path();
                        // Leftover directories can contain only app-generated data
                        // (e.g. mounted volumes) after an app was removed. They are
                        // not apps and must not be reported as such.
                        if !app_dir.join("compose.yaml").exists() {
                            continue;
                        }
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

        // 4. Check Cron apps
        let cron_apps = {
            let lock = self.installed_cron_apps.lock().await;
            lock.clone()
        };

        if !cron_apps.is_empty() {
            let current_crontab = crate::crontab::read_system_crontab().await.unwrap_or_default();
            let current_block = crate::crontab::extract_cron_block(&current_crontab).unwrap_or_default();

            for app in cron_apps {
                if seen.insert(app.name.clone()) {
                    let (state, text) = if !current_block.is_empty() {
                        if app.enabled {
                            ("running".to_string(), "active (scheduled in crontab)".to_string())
                        } else {
                            ("stopped".to_string(), "disabled (commented in crontab)".to_string())
                        }
                    } else {
                        ("stopped".to_string(), "not found in crontab".to_string())
                    };

                    statuses.push(AppStatus {
                        app_id: app.id.clone(),
                        name: app.name.clone(),
                        app_type: AppType::Cron as i32,
                        state,
                        status_text: text,
                        last_updated: chrono_now(),
                        error_message: String::new(),
                    });
                }
            }
        }

        statuses
    }

    async fn probe_docker_status(&self, name: &str) -> (String, String, String) {
        let project_name = format!("ezapp-{}", name);
        let compose_cmd = compose_command();
        let app_dir = PathBuf::from(DOCKER_BASE_DIR).join(name);

        let cmd = format!(
            "cd {} && {} -p {} ps --format '{{{{.State}}}} {{{{.Status}}}}' 2>/dev/null",
            shell_quote(app_dir.to_str().unwrap_or("")),
            compose_cmd,
            shell_quote(&project_name)
        );

        let (success, _, stdout, stderr) = run_shell(&cmd).await;
        let is_starting = {
            let starting = self.starting_apps.lock().await;
            starting.contains_key(name)
        };
        let last_error = {
            let errors = self.app_errors.lock().await;
            errors.get(name).cloned().unwrap_or_default()
        };

        if !success || stdout.trim().is_empty() {
            if is_starting {
                return (
                    "starting".to_string(),
                    "pulling images / starting containers...".to_string(),
                    String::new(),
                );
            }
            if !last_error.is_empty() {
                return ("failed".to_string(), "startup failed".to_string(), last_error);
            }
            return ("stopped".to_string(), "stopped or not started".to_string(), stderr);
        }

        let first_line = stdout.lines().next().unwrap_or("").trim();
        if first_line.to_lowercase().starts_with("running") {
            if is_starting {
                let mut starting = self.starting_apps.lock().await;
                starting.remove(name);
            }
            ("running".to_string(), first_line.to_string(), String::new())
        } else if is_starting {
            (
                "starting".to_string(),
                "pulling images / starting containers...".to_string(),
                String::new(),
            )
        } else if !last_error.is_empty() {
            ("failed".to_string(), first_line.to_string(), last_error)
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

        if app_type == AppType::Cron {
            return (false, -1, String::new(), "cron apps do not support action execution".to_string());
        }

        let action = req.action.to_lowercase();
        let lines = if req.log_lines > 0 { req.log_lines } else { 100 };

        if action == "stop" {
            let mut starting = self.starting_apps.lock().await;
            starting.remove(&req.app_name);
        }

        match app_type {
            AppType::DockerCompose => {
                let app_dir = PathBuf::from(DOCKER_BASE_DIR).join(&req.app_name);
                let project_name = format!("ezapp-{}", req.app_name);
                let compose_cmd = compose_command();

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
            AppType::Cron => (false, -1, String::new(), "cron apps do not support action execution".to_string()),
        }
    }
}

pub fn shell_quote(s: &str) -> String {
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

/// A relative path that is safe to join onto an app directory. The managed
/// manifest name is reserved for Easy42 itself.
fn is_safe_rel_path(rel_path: &str) -> bool {
    !rel_path.is_empty()
        && !rel_path.contains("..")
        && !rel_path.starts_with('/')
        && rel_path != MANAGED_MANIFEST_FILE
}

fn read_managed_manifest(base_dir: &Path) -> HashSet<String> {
    let mut managed = HashSet::new();
    let Ok(content) = fs::read_to_string(base_dir.join(MANAGED_MANIFEST_FILE)) else {
        return managed;
    };
    let Ok(manifest) = serde_json::from_str::<ManagedFilesManifest>(&content) else {
        return managed;
    };
    for rel_path in manifest.files {
        let rel_path = rel_path.trim().to_string();
        if is_safe_rel_path(&rel_path) {
            managed.insert(rel_path);
        }
    }
    managed
}

fn write_managed_manifest(base_dir: &Path, files: &HashSet<String>) -> Result<(), String> {
    let manifest_path = base_dir.join(MANAGED_MANIFEST_FILE);
    if files.is_empty() {
        match fs::remove_file(&manifest_path) {
            Ok(()) => {}
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
            Err(e) => return Err(format!("failed to remove managed file manifest {:?}: {}", manifest_path, e)),
        }
        return Ok(());
    }

    let mut sorted_files: Vec<&String> = files.iter().collect();
    sorted_files.sort();
    let manifest = ManagedFilesManifest {
        files: sorted_files.into_iter().cloned().collect(),
    };
    let content = serde_json::to_string_pretty(&manifest)
        .map_err(|e| format!("failed to serialize managed file manifest: {}", e))?;

    let tmp_path = base_dir.join(format!("{}.tmp", MANAGED_MANIFEST_FILE));
    fs::write(&tmp_path, content)
        .map_err(|e| format!("failed to write managed file manifest {:?}: {}", tmp_path, e))?;
    fs::rename(&tmp_path, &manifest_path)
        .map_err(|e| format!("failed to install managed file manifest {:?}: {}", manifest_path, e))?;
    Ok(())
}

/// Read only the files Easy42 wrote for an app, never arbitrary files that the
/// app itself may have created inside its directory.
fn read_managed_files(base_dir: &Path) -> Vec<(String, String)> {
    let mut files = Vec::new();
    for rel_path in read_managed_manifest(base_dir) {
        let path = base_dir.join(&rel_path);
        if path.is_file() {
            if let Ok(content) = fs::read_to_string(&path) {
                files.push((rel_path, content));
            }
        }
    }
    files.sort_by(|a, b| a.0.cmp(&b.0));
    files
}

/// Remove empty directories between a deleted managed file and the app base
/// directory. Never removes the base directory itself.
fn prune_empty_parent_dirs(base_dir: &Path, target: &Path) {
    let mut current = target.parent();
    while let Some(dir) = current {
        if dir == base_dir || !dir.starts_with(base_dir) {
            break;
        }
        if fs::remove_dir(dir).is_err() {
            break;
        }
        current = dir.parent();
    }
}

/// Remove only the files Easy42 generated for an app, then remove the app
/// directory itself only if it is empty. App-generated data is preserved.
fn remove_managed_app_files(base_dir: &Path, standard_rel_paths: &[&str]) {
    let mut targets = read_managed_manifest(base_dir);
    for rel_path in standard_rel_paths {
        if !rel_path.is_empty() {
            targets.insert((*rel_path).to_string());
        }
    }

    let mut targets: Vec<String> = targets.into_iter().collect();
    // Delete deeper paths first so their parent directories can be pruned.
    targets.sort_by(|a, b| {
        b.matches('/')
            .count()
            .cmp(&a.matches('/').count())
            .then_with(|| a.cmp(b))
    });

    for rel_path in &targets {
        if !is_safe_rel_path(rel_path) {
            continue;
        }
        let target_path = base_dir.join(rel_path);
        match fs::remove_file(&target_path) {
            Ok(()) => info!("Removed Easy42 managed app file: {:?}", target_path),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
            Err(e) => error!("Failed to remove Easy42 managed app file {:?}: {}", target_path, e),
        }
        prune_empty_parent_dirs(base_dir, &target_path);
    }

    let manifest_path = base_dir.join(MANAGED_MANIFEST_FILE);
    match fs::remove_file(&manifest_path) {
        Ok(()) => {}
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
        Err(e) => error!("Failed to remove managed file manifest {:?}: {}", manifest_path, e),
    }

    match fs::remove_dir(base_dir) {
        Ok(()) => info!("Removed empty app directory {:?}", base_dir),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
        Err(_) => info!("Preserving non-empty app directory {:?}", base_dir),
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

    #[test]
    fn test_remove_managed_app_files_preserves_user_data() {
        let tmp = tempfile::tempdir().unwrap();
        let base = tmp.path().join("app");
        fs::create_dir_all(base.join("conf")).unwrap();
        fs::create_dir_all(base.join("volume")).unwrap();
        fs::write(base.join("compose.yaml"), "services: {}").unwrap();
        fs::write(base.join(".env"), "A=1").unwrap();
        fs::write(base.join("conf/rclone.conf"), "cfg").unwrap();
        fs::write(base.join("volume/data.db"), "user data").unwrap();

        let mut managed = HashSet::new();
        managed.insert("conf/rclone.conf".to_string());
        write_managed_manifest(&base, &managed).unwrap();

        remove_managed_app_files(&base, &["compose.yaml", ".env"]);

        assert!(!base.join("compose.yaml").exists());
        assert!(!base.join(".env").exists());
        assert!(!base.join("conf").exists());
        assert!(!base.join(MANAGED_MANIFEST_FILE).exists());
        assert_eq!(
            fs::read_to_string(base.join("volume/data.db")).unwrap(),
            "user data"
        );
        assert!(base.is_dir(), "directory must be preserved while user data remains");
    }

    #[test]
    fn test_remove_managed_app_files_removes_empty_dir() {
        let tmp = tempfile::tempdir().unwrap();
        let base = tmp.path().join("app");
        fs::create_dir_all(base.join("conf")).unwrap();
        fs::write(base.join("compose.yaml"), "services: {}").unwrap();
        fs::write(base.join(".env"), "A=1").unwrap();
        fs::write(base.join("conf/rclone.conf"), "cfg").unwrap();

        let mut managed = HashSet::new();
        managed.insert("conf/rclone.conf".to_string());
        write_managed_manifest(&base, &managed).unwrap();

        remove_managed_app_files(&base, &["compose.yaml", ".env"]);

        assert!(!base.exists(), "empty app directory should be removed");
    }

    #[test]
    fn test_remove_managed_app_files_without_manifest_keeps_user_data() {
        let tmp = tempfile::tempdir().unwrap();
        let base = tmp.path().join("app");
        fs::create_dir_all(&base).unwrap();
        fs::write(base.join("compose.yaml"), "services: {}").unwrap();
        fs::write(base.join(".env"), "A=1").unwrap();
        fs::write(base.join("user.txt"), "user data").unwrap();

        remove_managed_app_files(&base, &["compose.yaml", ".env"]);

        assert!(!base.join("compose.yaml").exists());
        assert!(!base.join(".env").exists());
        assert_eq!(fs::read_to_string(base.join("user.txt")).unwrap(), "user data");
        assert!(base.is_dir());
    }

    #[test]
    fn test_sync_config_files_only_removes_managed_files() {
        let tmp = tempfile::tempdir().unwrap();
        let base = tmp.path().join("app");
        fs::create_dir_all(base.join("conf")).unwrap();
        fs::write(base.join("conf/old.conf"), "old").unwrap();
        fs::write(base.join("data.bin"), "user data").unwrap();

        let mut managed = HashSet::new();
        managed.insert("conf/old.conf".to_string());
        write_managed_manifest(&base, &managed).unwrap();

        let manager = AppsManager::new();
        let files = vec![AppConfigFile {
            path: "conf/new.conf".to_string(),
            content: b"new".to_vec(),
            file_mode: 0o644,
        }];
        manager.sync_config_files(&base, &files, &[]).unwrap();

        assert!(!base.join("conf/old.conf").exists());
        assert_eq!(
            fs::read_to_string(base.join("conf/new.conf")).unwrap(),
            "new"
        );
        assert_eq!(fs::read_to_string(base.join("data.bin")).unwrap(), "user data");

        let managed_after = read_managed_manifest(&base);
        assert!(managed_after.contains("conf/new.conf"));
        assert!(!managed_after.contains("conf/old.conf"));
    }

    #[test]
    fn test_sync_config_files_empty_preserves_user_data() {
        let tmp = tempfile::tempdir().unwrap();
        let base = tmp.path().join("app-data");
        fs::create_dir_all(base.join("conf")).unwrap();
        fs::write(base.join("conf/old.conf"), "old").unwrap();
        fs::write(base.join("volume.db"), "user data").unwrap();

        let mut managed = HashSet::new();
        managed.insert("conf/old.conf".to_string());
        write_managed_manifest(&base, &managed).unwrap();

        let manager = AppsManager::new();
        manager.sync_config_files(&base, &[], &[]).unwrap();

        assert!(!base.join("conf").exists());
        assert!(!base.join(MANAGED_MANIFEST_FILE).exists());
        assert_eq!(
            fs::read_to_string(base.join("volume.db")).unwrap(),
            "user data"
        );
        assert!(base.is_dir());
    }

    #[test]
    fn test_read_managed_files_ignores_unmanaged_files() {
        let tmp = tempfile::tempdir().unwrap();
        let base = tmp.path().join("app");
        fs::create_dir_all(base.join("conf")).unwrap();
        fs::write(base.join("conf/app.conf"), "managed").unwrap();
        fs::write(base.join("user-data.bin"), "user data").unwrap();

        let mut managed = HashSet::new();
        managed.insert("conf/app.conf".to_string());
        write_managed_manifest(&base, &managed).unwrap();

        let files = read_managed_files(&base);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0].0, "conf/app.conf");
        assert_eq!(files[0].1, "managed");
    }
}


