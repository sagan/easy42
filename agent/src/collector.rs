use std::collections::HashMap;
use std::ffi::CString;
use std::fs;
use std::mem::MaybeUninit;
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
    PointToPoint,
    Other,
}

impl InterfaceCategory {
    pub fn is_reported(&self) -> bool {
        matches!(
            self,
            InterfaceCategory::Physical
                | InterfaceCategory::Tunnel
                | InterfaceCategory::PointToPoint
        )
    }
}

pub const IFACE_FLAG_PRIMARY: u32 = 1 << 0;  // Bit 0: Primary interface
pub const IFACE_FLAG_PHYSICAL: u32 = 1 << 1; // Bit 1: Physical interface

pub struct MetricsCollector {
    prev_cpu_idle: u64,
    prev_cpu_total: u64,
    interface_types: HashMap<String, InterfaceCategory>,
    cached_primary_iface: Option<String>,
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
            cached_primary_iface: None,
        }
    }

    pub fn flush_interface_cache(&mut self) {
        self.interface_types.clear();
        self.cached_primary_iface = None;
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
            disks: self.collect_disks(),
        }
    }

    pub fn collect_disks(&self) -> Vec<proto::DiskMetrics> {
        let mut disks = Vec::new();
        // Hardcoded to only collect root path "/" for now, structured as a list for future mount points
        if let Some(root_disk) = collect_disk_usage("/") {
            disks.push(root_disk);
        }
        disks
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

        let primary_iface = match &self.cached_primary_iface {
            Some(name) => Some(name.clone()),
            None => {
                let detected = detect_primary_interface().await;
                self.cached_primary_iface = detected.clone();
                detected
            }
        };

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

                    let mut flags = 0u32;
                    if let Some(ref pri) = primary_iface {
                        if &iface_name == pri {
                            flags |= IFACE_FLAG_PRIMARY;
                        }
                    }
                    if is_physical_interface(&iface_name, "/sys/class/net")
                        || category == InterfaceCategory::Physical
                    {
                        flags |= IFACE_FLAG_PHYSICAL;
                    }

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
                        flags,
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
        Some(512) => InterfaceCategory::PointToPoint,
        Some(772) => InterfaceCategory::Loopback,
        Some(65534) => InterfaceCategory::Tunnel,
        _ => InterfaceCategory::Other,
    }
}

pub fn is_physical_interface<P: AsRef<Path>>(name: &str, sys_net_dir: P) -> bool {
    let iface_dir = sys_net_dir.as_ref().join(name);
    let device_path = iface_dir.join("device");
    device_path.exists() || fs::symlink_metadata(&device_path).is_ok()
}

pub async fn detect_primary_interface() -> Option<String> {
    if let Some(stdout) = run_cmd_timed("ip", &["route", "show", "default"], std::time::Duration::from_secs(3)).await {
        parse_default_route_interface(&stdout)
    } else {
        None
    }
}

pub fn parse_default_route_interface(output: &str) -> Option<String> {
    for line in output.lines() {
        let parts: Vec<&str> = line.split_whitespace().collect();
        if parts.first() != Some(&"default") {
            continue;
        }
        if let Some(pos) = parts.iter().position(|&x| x == "dev") {
            if let Some(iface) = parts.get(pos + 1) {
                return Some(iface.to_string());
            }
        }
    }
    None
}

pub fn collect_disk_usage(path: &str) -> Option<proto::DiskMetrics> {
    let c_path = CString::new(path).ok()?;
    let mut stat = MaybeUninit::<libc::statvfs>::uninit();

    let res = unsafe { libc::statvfs(c_path.as_ptr(), stat.as_mut_ptr()) };
    if res != 0 {
        tracing::warn!("statvfs failed for path: {}", path);
        return None;
    }

    let stat = unsafe { stat.assume_init() };
    let block_size = if stat.f_frsize > 0 {
        stat.f_frsize as u64
    } else {
        stat.f_bsize as u64
    };

    let total_blocks = stat.f_blocks as u64;
    let free_blocks = stat.f_bfree as u64;
    let avail_blocks = stat.f_bavail as u64;

    let total_bytes = total_blocks.saturating_mul(block_size);
    let used_blocks = total_blocks.saturating_sub(free_blocks);
    let used_bytes = used_blocks.saturating_mul(block_size);
    let free_bytes = avail_blocks.saturating_mul(block_size);

    Some(proto::DiskMetrics {
        path: path.to_string(),
        total_bytes,
        used_bytes,
        free_bytes,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_collect_disk_usage_root() {
        let usage = collect_disk_usage("/");
        assert!(usage.is_some(), "collect_disk_usage('/') should succeed on Linux");
        let usage = usage.unwrap();
        assert_eq!(usage.path, "/");
        assert!(usage.total_bytes > 0, "total_bytes should be greater than 0");
        assert!(usage.used_bytes > 0, "used_bytes should be greater than 0");
        assert!(usage.total_bytes >= usage.used_bytes, "total_bytes >= used_bytes");
    }

    #[test]
    fn test_collect_disk_usage_nonexistent() {
        let usage = collect_disk_usage("/nonexistent_path_that_should_not_exist_4242");
        assert!(usage.is_none());
    }

    #[test]
    fn test_collector_collect_disks() {
        let collector = MetricsCollector::new();
        let disks = collector.collect_disks();
        assert_eq!(disks.len(), 1);
        assert_eq!(disks[0].path, "/");
        assert!(disks[0].total_bytes > 0);
    }

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

        // 5. Point-to-Point device: type 512
        let ppp_dir = temp_dir.join("ppp0");
        fs::create_dir_all(&ppp_dir).unwrap();
        fs::write(ppp_dir.join("type"), "512\n").unwrap();
        assert_eq!(
            categorize_interface_with_sysfs("ppp0", &temp_dir),
            InterfaceCategory::PointToPoint
        );
        assert!(InterfaceCategory::PointToPoint.is_reported());

        // 6. Unknown / other
        let sit_dir = temp_dir.join("sit0");
        fs::create_dir_all(&sit_dir).unwrap();
        fs::write(sit_dir.join("type"), "776\n").unwrap();
        assert_eq!(
            categorize_interface_with_sysfs("sit0", &temp_dir),
            InterfaceCategory::Other
        );
        assert!(!InterfaceCategory::Other.is_reported());

        // 7. Non-existent interface
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

        // Test flush_interface_cache
        collector.flush_interface_cache();
        assert!(collector.interface_types.is_empty());
        assert!(collector.cached_primary_iface.is_none());
    }

    #[test]
    fn test_parse_default_route() {
        let route_sample1 = "default via 172.24.3.254 dev ens3 proto static\n";
        assert_eq!(parse_default_route_interface(route_sample1), Some("ens3".to_string()));

        let route_sample2 = "default dev ppp0 scope link\n10.0.0.0/8 dev eth0 proto kernel\n";
        assert_eq!(parse_default_route_interface(route_sample2), Some("ppp0".to_string()));

        let route_sample3 = "10.0.0.0/8 dev eth0 proto kernel\n";
        assert_eq!(parse_default_route_interface(route_sample3), None);
    }
}

