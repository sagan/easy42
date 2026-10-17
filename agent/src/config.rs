use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentConfig {
    pub server: String,
    pub token: String,
    #[serde(default = "default_log_level")]
    pub log_level: String,
    #[serde(default = "default_telemetry_interval")]
    pub telemetry_interval_secs: u64,
    #[serde(default)]
    pub node_name: Option<String>,
    /// Optional shell script snippet executed before Sync action
    #[serde(default)]
    pub pre_sync: Option<String>,
    /// Optional shell script snippet executed after Sync action
    #[serde(default)]
    pub post_sync: Option<String>,
    /// Path to config file if loaded from disk
    #[serde(skip)]
    pub config_path: Option<PathBuf>,
}

fn default_log_level() -> String {
    "info".to_string()
}

fn default_telemetry_interval() -> u64 {
    10
}

impl Default for AgentConfig {
    fn default() -> Self {
        Self {
            server: String::new(),
            token: String::new(),
            log_level: default_log_level(),
            telemetry_interval_secs: default_telemetry_interval(),
            node_name: None,
            pre_sync: None,
            post_sync: None,
            config_path: None,
        }
    }
}

impl AgentConfig {
    pub fn load_from_file<P: AsRef<Path>>(path: P) -> Result<Self, Box<dyn std::error::Error>> {
        let content = fs::read_to_string(path.as_ref())?;
        let mut config: AgentConfig = serde_json::from_str(&content)?;
        config.config_path = Some(path.as_ref().to_path_buf());
        Ok(config)
    }

    pub fn pre_sync_hook(&self) -> Option<&str> {
        self.pre_sync
            .as_deref()
            .filter(|s| !s.trim().is_empty())
    }

    pub fn post_sync_hook(&self) -> Option<&str> {
        self.post_sync
            .as_deref()
            .filter(|s| !s.trim().is_empty())
    }

    pub fn server_ws_url(&self) -> String {
        let trimmed = self.server.trim_end_matches('/');
        let ws_base = if trimmed.starts_with("https://") {
            trimmed.replacen("https://", "wss://", 1)
        } else if trimmed.starts_with("http://") {
            trimmed.replacen("http://", "ws://", 1)
        } else if trimmed.starts_with("wss://") || trimmed.starts_with("ws://") {
            trimmed.to_string()
        } else {
            format!("wss://{}", trimmed)
        };
        format!("{}/api/agent/ws", ws_base)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_config_without_hooks() {
        let json = r#"{"server":"https://easy42.local","token":"abc"}"#;
        let cfg: AgentConfig = serde_json::from_str(json).unwrap();
        assert_eq!(cfg.server, "https://easy42.local");
        assert_eq!(cfg.token, "abc");
        assert_eq!(cfg.log_level, "info");
        assert_eq!(cfg.telemetry_interval_secs, 10);
        assert_eq!(cfg.pre_sync_hook(), None);
        assert_eq!(cfg.post_sync_hook(), None);
    }

    #[test]
    fn test_config_with_hooks() {
        let json = r#"{
            "server": "https://easy42.local",
            "token": "abc",
            "pre_sync": "echo pre >> /tmp/hook.log",
            "post_sync": "echo post >> /tmp/hook.log"
        }"#;
        let cfg: AgentConfig = serde_json::from_str(json).unwrap();
        assert_eq!(cfg.pre_sync_hook(), Some("echo pre >> /tmp/hook.log"));
        assert_eq!(cfg.post_sync_hook(), Some("echo post >> /tmp/hook.log"));
    }

    #[test]
    fn test_config_empty_hooks_ignored() {
        let json = r#"{
            "server": "https://easy42.local",
            "token": "abc",
            "pre_sync": "   ",
            "post_sync": ""
        }"#;
        let cfg: AgentConfig = serde_json::from_str(json).unwrap();
        assert_eq!(cfg.pre_sync_hook(), None);
        assert_eq!(cfg.post_sync_hook(), None);
    }
}
