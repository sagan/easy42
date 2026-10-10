use std::collections::HashMap;
use std::fs;
use std::path::Path;

pub mod proto {
    include!(concat!(env!("OUT_DIR"), "/easy42.agent.v1.rs"));
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum InterfaceCategory {
    Physical,
    Loopback,
    BridgeOrEphemeral,
    Tunnel,
    Other,
}

impl InterfaceCategory {
    pub fn is_reported(&self) -> bool {
        matches!(self, InterfaceCategory::Physical | InterfaceCategory::Tunnel)
    }
}

pub struct MetricsCollector {
    prev_cpu_idle: u64,
    prev_cpu_total: u64,
    interface_types: HashMap<String, InterfaceCategory>,
}

impl Default for MetricsCollector {
    fn default() -> Self {
        Self::new()
    }
}

impl MetricsCollector {
    pub fn new() -> Self {
        Self {
            prev_cpu_idle: 0,
            prev_cpu_total: 0,
            interface_types: HashMap::new(),
        }
    }

    #[allow(dead_code)]
    pub fn interface_category(&self, name: &str) -> Option<InterfaceCategory> {
        self.interface_types.get(name).copied()
    }

    pub async fn collect_telemetry(&mut self) -> proto::TelemetryReport {
        proto::TelemetryReport {
            system: Some(self.collect_system_metrics()),
            interfaces: self.collect_interfaces().await,
            wg_peers: self.collect_wg_peers().await,
            bird_protocols: self.collect_bird_protocols().await,
        }
    }

    pub fn collect_system_metrics(&mut self) -> proto::SystemMetrics {
        let (cpu_percent, idle, total) = self.calculate_cpu_percent();
        self.prev_cpu_idle = idle;
        self.prev_cpu_total = total;

        let (mem_used, mem_total) = self.read_memory();
        let uptime = self.read_uptime();
        let load_avg = self.read_load_avg();

        proto::SystemMetrics {
            cpu_percent,
            memory_used_bytes: mem_used,
            memory_total_bytes: mem_total,
            uptime_seconds: uptime,
            load_avg,
        }
    }

    fn calculate_cpu_percent(&self) -> (f32, u64, u64) {
        if let Ok(stat) = fs::read_to_string("/proc/stat") {
            if let Some(line) = stat.lines().next() {
                let parts: Vec<&str> = line.split_whitespace().collect();
                if parts.len() >= 5 {
                    let user: u64 = parts[1].parse().unwrap_or(0);
                    let nice: u64 = parts[2].parse().unwrap_or(0);
                    let system: u64 = parts[3].parse().unwrap_or(0);
                    let idle: u64 = parts[4].parse().unwrap_or(0);
                    let iowait: u64 = parts.get(5).and_then(|v| v.parse().ok()).unwrap_or(0);
                    let irq: u64 = parts.get(6).and_then(|v| v.parse().ok()).unwrap_or(0);
                    let softirq: u64 = parts.get(7).and_then(|v| v.parse().ok()).unwrap_or(0);
                    let steal: u64 = parts.get(8).and_then(|v| v.parse().ok()).unwrap_or(0);

                    let total = user + nice + system + idle + iowait + irq + softirq + steal;
                    let idle_total = idle + iowait;

                    if self.prev_cpu_total > 0 && total > self.prev_cpu_total {
                        let total_diff = (total - self.prev_cpu_total) as f32;
                        let idle_diff = (idle_total.saturating_sub(self.prev_cpu_idle)) as f32;
                        let usage = 100.0 * (1.0 - (idle_diff / total_diff));
                        return (usage.clamp(0.0, 100.0), idle_total, total);
                    }
                    return (0.0, idle_total, total);
                }
            }
        }
        (0.0, 0, 0)
    }

    fn read_memory(&self) -> (u64, u64) {
        let mut total = 0;
        let mut available = 0;

        if let Ok(meminfo) = fs::read_to_string("/proc/meminfo") {
            for line in meminfo.lines() {
                if line.starts_with("MemTotal:") {
                    total = parse_mem_kb(line) * 1024;
                } else if line.starts_with("MemAvailable:") {
                    available = parse_mem_kb(line) * 1024;
                }
            }
        }

        let used = total.saturating_sub(available);
        (used, total)
    }

    fn read_uptime(&self) -> u64 {
        if let Ok(content) = fs::read_to_string("/proc/uptime") {
            if let Some(first) = content.split_whitespace().next() {
                if let Ok(secs) = first.parse::<f64>() {
                    return secs as u64;
                }
            }
        }
        0
    }

    fn read_load_avg(&self) -> Vec<f32> {
        if let Ok(content) = fs::read_to_string("/proc/loadavg") {
            let parts: Vec<&str> = content.split_whitespace().collect();
            if parts.len() >= 3 {
                let l1 = parts[0].parse::<f32>().unwrap_or(0.0);
                let l5 = parts[1].parse::<f32>().unwrap_or(0.0);
                let l15 = parts[2].parse::<f32>().unwrap_or(0.0);
                return vec![l1, l5, l15];
            }
        }
        vec![0.0, 0.0, 0.0]
    }

    pub async fn collect_interfaces(&mut self) -> Vec<proto::InterfaceMetrics> {
        let mut list = Vec::new();
        let addr_map = get_interface_addresses().await;

        if let Ok(dev) = fs::read_to_string("/proc/net/dev") {
            for line in dev.lines().skip(2) {
                let mut parts = line.split_whitespace();
                if let Some(name_col) = parts.next() {
                    let iface_name = name_col.trim_end_matches(':').to_string();

                    let category = *self
                        .interface_types
                        .entry(iface_name.clone())
                        .or_insert_with(|| {
                            let cat = categorize_interface(&iface_name);
                            tracing::debug!(
                                "Interface '{}' categorized as {:?}",
                                iface_name,
                                cat
                            );
                            cat
                        });

                    if !category.is_reported() {
                        continue;
                    }

                    let rx_bytes = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0);
                    let rx_packets = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0);
                    let rx_errors = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0);
                    let _drop = parts.next();
                    let _fifo = parts.next();
                    let _frame = parts.next();
                    let _compressed = parts.next();
                    let _multicast = parts.next();
                    let tx_bytes = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0);
                    let tx_packets = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0);
                    let tx_errors = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0);

                    let is_up = check_interface_up(&iface_name);
                    let addresses = addr_map.get(&iface_name).cloned().unwrap_or_default();

                    list.push(proto::InterfaceMetrics {
                        name: iface_name,
                        is_up,
                        rx_bytes,
                        tx_bytes,
                        rx_packets,
                        tx_packets,
                        rx_errors,
                        tx_errors,
                        addresses,
                    });
                }
            }
        }
        list
    }

    pub async fn collect_wg_peers(&self) -> Vec<proto::WgPeerMetrics> {
        let mut peers = Vec::new();

        // Run `wg show all dump` with 3-second timeout
        if let Some(text) = run_cmd_timed("wg", &["show", "all", "dump"], std::time::Duration::from_secs(3)).await {
            for line in text.lines() {
                let parts: Vec<&str> = line.split('\t').collect();
                // Peer format: <interface> <public-key> <preshared-key> <endpoint> <allowed-ips> <latest-handshake> <transfer-rx> <transfer-tx> <persistent-keepalive>
                if parts.len() >= 9 {
                    let iface = parts[0].to_string();
                    let pubkey = parts[1].to_string();
                    let endpoint = parts[3].to_string();
                    let handshake = parts[5].parse::<i64>().unwrap_or(0);
                    let rx = parts[6].parse::<u64>().unwrap_or(0);
                    let tx = parts[7].parse::<u64>().unwrap_or(0);
                    let keepalive = parts[8].parse::<i64>().unwrap_or(0);

                    peers.push(proto::WgPeerMetrics {
                        interface_name: iface,
                        public_key: pubkey,
                        endpoint: if endpoint == "(none)" {
                            "".to_string()
                        } else {
                            endpoint
                        },
                        last_handshake_time: handshake,
                        rx_bytes: rx,
                        tx_bytes: tx,
                        persistent_keepalive: keepalive,
                    });
                }
            }
        }
        peers
    }

    pub async fn collect_bird_protocols(&self) -> Vec<proto::BirdProtocolStatus> {
        let mut protocols = Vec::new();

        // Run `birdc show protocols` with 3-second timeout
        if let Some(text) = run_cmd_timed("birdc", &["show", "protocols"], std::time::Duration::from_secs(3)).await {
            for line in text.lines().skip(1) {
                // Skip header: Name Proto Table State Since Info
                let parts: Vec<&str> = line.split_whitespace().collect();
                if parts.len() >= 5 {
                    let name = parts[0].to_string();
                    let proto = parts[1].to_string();
                    let state = parts[3].to_string();
                    let info = parts[4..].join(" ");

                    protocols.push(proto::BirdProtocolStatus {
                        name,
                        proto,
                        state,
                        info,
                    });
                }
            }
        }
        protocols
    }
}

fn parse_mem_kb(line: &str) -> u64 {
    line.split_whitespace()
        .nth(1)
        .and_then(|v| v.parse().ok())
        .unwrap_or(0)
}

fn check_interface_up(name: &str) -> bool {
    let path = format!("/sys/class/net/{}/operstate", name);
    if let Ok(state) = fs::read_to_string(path) {
        let s = state.trim();
        return s.eq_ignore_ascii_case("up") || s.eq_ignore_ascii_case("unknown");
    }
    true
}

async fn get_interface_addresses() -> HashMap<String, Vec<String>> {
    let mut map: HashMap<String, Vec<String>> = HashMap::new();
    if let Some(stdout_str) = run_cmd_timed("ip", &["-j", "addr", "show"], std::time::Duration::from_secs(3)).await {
        if let Ok(val) = serde_json::from_str::<serde_json::Value>(&stdout_str) {
            if let Some(arr) = val.as_array() {
                for iface in arr {
                    let name = iface["ifname"].as_str().unwrap_or("").to_string();
                    let mut addrs = Vec::new();
                    if let Some(addr_info) = iface["addr_info"].as_array() {
                        for a in addr_info {
                            if let (Some(local), Some(prefix)) =
                                (a["local"].as_str(), a["prefixlen"].as_u64())
                            {
                                addrs.push(format!("{}/{}", local, prefix));
                            }
                        }
                    }
                    map.insert(name, addrs);
                }
            }
        }
    }
    map
}

async fn run_cmd_timed(program: &str, args: &[&str], timeout_dur: std::time::Duration) -> Option<String> {
    use tokio::process::Command;
    let mut cmd = Command::new(program);
    cmd.args(args);
    match tokio::time::timeout(timeout_dur, cmd.output()).await {
        Ok(Ok(output)) if output.status.success() => {
            Some(String::from_utf8_lossy(&output.stdout).to_string())
        }
        _ => None,
    }
}

pub fn categorize_interface(name: &str) -> InterfaceCategory {
    categorize_interface_with_sysfs(name, "/sys/class/net")
}

pub fn categorize_interface_with_sysfs<P: AsRef<Path>>(
    name: &str,
    sys_net_dir: P,
) -> InterfaceCategory {
    let iface_dir = sys_net_dir.as_ref().join(name);
    let type_path = iface_dir.join("type");
    let device_path = iface_dir.join("device");

    let iface_type = fs::read_to_string(&type_path)
        .ok()
        .and_then(|s| s.trim().parse::<u32>().ok());

    let device_exists = device_path.exists() || fs::symlink_metadata(&device_path).is_ok();

    match iface_type {
        Some(1) => {
            if device_exists {
                InterfaceCategory::Physical
            } else {
                InterfaceCategory::BridgeOrEphemeral
            }
        }
        Some(772) => InterfaceCategory::Loopback,
        Some(65534) => InterfaceCategory::Tunnel,
        _ => InterfaceCategory::Other,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_categorize_interface_types() {
        let temp_dir = std::env::temp_dir().join(format!("test_easy42_sysfs_{}", std::process::id()));
        let _ = fs::remove_dir_all(&temp_dir);
        fs::create_dir_all(&temp_dir).unwrap();

        // 1. Physical device: type 1, device exists
        let eth_dir = temp_dir.join("eth0");
        fs::create_dir_all(&eth_dir).unwrap();
        fs::write(eth_dir.join("type"), "1\n").unwrap();
        fs::create_dir_all(eth_dir.join("device")).unwrap();
        assert_eq!(
            categorize_interface_with_sysfs("eth0", &temp_dir),
            InterfaceCategory::Physical
        );
        assert!(InterfaceCategory::Physical.is_reported());

        // 2. Loopback: type 772
        let lo_dir = temp_dir.join("lo");
        fs::create_dir_all(&lo_dir).unwrap();
        fs::write(lo_dir.join("type"), "772\n").unwrap();
        assert_eq!(
            categorize_interface_with_sysfs("lo", &temp_dir),
            InterfaceCategory::Loopback
        );
        assert!(!InterfaceCategory::Loopback.is_reported());

        // 3. Bridge or ephemeral device: type 1, device missing, bridge may exist
        let br_dir = temp_dir.join("docker0");
        fs::create_dir_all(&br_dir).unwrap();
        fs::write(br_dir.join("type"), "1\n").unwrap();
        fs::create_dir_all(br_dir.join("bridge")).unwrap();
        assert_eq!(
            categorize_interface_with_sysfs("docker0", &temp_dir),
            InterfaceCategory::BridgeOrEphemeral
        );
        assert!(!InterfaceCategory::BridgeOrEphemeral.is_reported());

        // Ephemeral veth: type 1, device missing, no bridge
        let veth_dir = temp_dir.join("veth12345");
        fs::create_dir_all(&veth_dir).unwrap();
        fs::write(veth_dir.join("type"), "1\n").unwrap();
        assert_eq!(
            categorize_interface_with_sysfs("veth12345", &temp_dir),
            InterfaceCategory::BridgeOrEphemeral
        );
        assert!(!InterfaceCategory::BridgeOrEphemeral.is_reported());

        // 4. Tunnel device: type 65534
        let wg_dir = temp_dir.join("wg42node");
        fs::create_dir_all(&wg_dir).unwrap();
        fs::write(wg_dir.join("type"), "65534\n").unwrap();
        assert_eq!(
            categorize_interface_with_sysfs("wg42node", &temp_dir),
            InterfaceCategory::Tunnel
        );
        assert!(InterfaceCategory::Tunnel.is_reported());

        // 5. Unknown / other
        let sit_dir = temp_dir.join("sit0");
        fs::create_dir_all(&sit_dir).unwrap();
        fs::write(sit_dir.join("type"), "776\n").unwrap();
        assert_eq!(
            categorize_interface_with_sysfs("sit0", &temp_dir),
            InterfaceCategory::Other
        );
        assert!(!InterfaceCategory::Other.is_reported());

        // 6. Non-existent interface
        assert_eq!(
            categorize_interface_with_sysfs("nonexistent", &temp_dir),
            InterfaceCategory::Other
        );
        assert!(!InterfaceCategory::Other.is_reported());

        let _ = fs::remove_dir_all(&temp_dir);
    }

    #[test]
    fn test_system_interfaces_classification() {
        if Path::new("/sys/class/net/lo").exists() {
            assert_eq!(categorize_interface("lo"), InterfaceCategory::Loopback);
        }
        if Path::new("/sys/class/net/ens3").exists() {
            assert_eq!(categorize_interface("ens3"), InterfaceCategory::Physical);
        }
        if Path::new("/sys/class/net/docker0").exists() {
            assert_eq!(categorize_interface("docker0"), InterfaceCategory::BridgeOrEphemeral);
        }
    }

    #[tokio::test]
    async fn test_collector_caching() {
        let mut collector = MetricsCollector::new();
        assert!(collector.interface_types.is_empty());

        let ifaces = collector.collect_interfaces().await;
        // On our system, only physical and tunnel interfaces should be collected
        for iface in &ifaces {
            let cat = collector.interface_category(&iface.name);
            assert!(matches!(cat, Some(InterfaceCategory::Physical) | Some(InterfaceCategory::Tunnel)));
            assert_ne!(iface.name, "lo");
            assert_ne!(iface.name, "docker0");
        }

        // Cache must contain categorized entries for all interfaces encountered in /proc/net/dev
        if Path::new("/sys/class/net/lo").exists() {
            assert_eq!(collector.interface_category("lo"), Some(InterfaceCategory::Loopback));
        }
        if Path::new("/sys/class/net/docker0").exists() {
            assert_eq!(collector.interface_category("docker0"), Some(InterfaceCategory::BridgeOrEphemeral));
        }
        if Path::new("/sys/class/net/ens3").exists() {
            assert_eq!(collector.interface_category("ens3"), Some(InterfaceCategory::Physical));
        }
    }
}

