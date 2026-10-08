use std::collections::HashMap;
use std::fs;
use std::process::Command;

pub mod proto {
    include!(concat!(env!("OUT_DIR"), "/easy42.agent.v1.rs"));
}

pub struct MetricsCollector {
    prev_cpu_idle: u64,
    prev_cpu_total: u64,
}

impl MetricsCollector {
    pub fn new() -> Self {
        Self {
            prev_cpu_idle: 0,
            prev_cpu_total: 0,
        }
    }

    pub fn collect_telemetry(&mut self) -> proto::TelemetryReport {
        proto::TelemetryReport {
            system: Some(self.collect_system_metrics()),
            interfaces: self.collect_interfaces(),
            wg_peers: self.collect_wg_peers(),
            bird_protocols: self.collect_bird_protocols(),
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

    pub fn collect_interfaces(&self) -> Vec<proto::InterfaceMetrics> {
        let mut list = Vec::new();
        let addr_map = get_interface_addresses();

        if let Ok(dev) = fs::read_to_string("/proc/net/dev") {
            for line in dev.lines().skip(2) {
                let mut parts = line.split_whitespace();
                if let Some(name_col) = parts.next() {
                    let iface_name = name_col.trim_end_matches(':').to_string();
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

    pub fn collect_wg_peers(&self) -> Vec<proto::WgPeerMetrics> {
        let mut peers = Vec::new();

        // Run `wg show all dump`
        if let Ok(output) = Command::new("wg").args(["show", "all", "dump"]).output() {
            if output.status.success() {
                let text = String::from_utf8_lossy(&output.stdout);
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
                            endpoint: if endpoint == "(none)" { "".to_string() } else { endpoint },
                            last_handshake_time: handshake,
                            rx_bytes: rx,
                            tx_bytes: tx,
                            persistent_keepalive: keepalive,
                        });
                    }
                }
            }
        }
        peers
    }

    pub fn collect_bird_protocols(&self) -> Vec<proto::BirdProtocolStatus> {
        let mut protocols = Vec::new();

        if let Ok(output) = Command::new("birdc").args(["show", "protocols"]).output() {
            if output.status.success() {
                let text = String::from_utf8_lossy(&output.stdout);
                for line in text.lines().skip(1) { // Skip header: Name Proto Table State Since Info
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

fn get_interface_addresses() -> HashMap<String, Vec<String>> {
    let mut map: HashMap<String, Vec<String>> = HashMap::new();
    if let Ok(output) = Command::new("ip").args(["-j", "addr", "show"]).output() {
        if output.status.success() {
            if let Ok(val) = serde_json::from_slice::<serde_json::Value>(&output.stdout) {
                if let Some(arr) = val.as_array() {
                    for iface in arr {
                        let name = iface["ifname"].as_str().unwrap_or("").to_string();
                        let mut addrs = Vec::new();
                        if let Some(addr_info) = iface["addr_info"].as_array() {
                            for a in addr_info {
                                if let (Some(local), Some(prefix)) = (a["local"].as_str(), a["prefixlen"].as_u64()) {
                                    addrs.push(format!("{}/{}", local, prefix));
                                }
                            }
                        }
                        map.insert(name, addrs);
                    }
                }
            }
        }
    }
    map
}
