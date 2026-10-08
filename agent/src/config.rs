use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;

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
}

fn default_log_level() -> String {
    "info".to_string()
}

fn default_telemetry_interval() -> u64 {
    10
}

impl AgentConfig {
    pub fn load_from_file<P: AsRef<Path>>(path: P) -> Result<Self, Box<dyn std::error::Error>> {
        let content = fs::read_to_string(path)?;
        let config: AgentConfig = toml::from_str(&content)?;
        Ok(config)
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
