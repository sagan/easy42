use std::net::{Ipv4Addr, Ipv6Addr};
use std::str::FromStr;

/// Validates interface names (1-15 characters, [a-zA-Z0-9_-])
pub fn is_valid_iface_name(iface: &str) -> bool {
    let s = iface.trim();
    !s.is_empty()
        && s.len() <= 15
        && s.chars().all(|c| c.is_ascii_alphanumeric() || c == '_' || c == '-')
}

/// Validates node names (1-64 characters, [a-zA-Z0-9_-])
pub fn is_valid_node_name(name: &str) -> bool {
    let s = name.trim();
    !s.is_empty()
        && s.len() <= 64
        && s.chars().all(|c| c.is_ascii_alphanumeric() || c == '_' || c == '-')
}

/// Checks if string is a valid IPv4 address (supports optional /prefix suffix)
pub fn is_valid_ipv4(s: &str) -> bool {
    let mut s = s.trim();
    if let Some(idx) = s.find('/') {
        s = &s[..idx];
    }
    Ipv4Addr::from_str(s).is_ok()
}

/// Checks if string is a valid IPv6 address (supports optional /prefix suffix)
pub fn is_valid_ipv6(s: &str) -> bool {
    let mut s = s.trim();
    if let Some(idx) = s.find('/') {
        s = &s[..idx];
    }
    Ipv6Addr::from_str(s).is_ok()
}

/// Checks if string is a valid IPv4 or IPv6 address
pub fn is_valid_ip(s: &str) -> bool {
    let s = s.trim();
    if is_valid_ipv4(s) {
        return true;
    }
    is_valid_ipv6(s)
}

/// Validates whether a string conforms to a valid domain / hostname (RFC 1035 / RFC 1123)
pub fn is_valid_domain(domain: &str) -> bool {
    let d = domain.trim().trim_end_matches('.');
    if d.is_empty() || d.len() > 253 {
        return false;
    }
    for label in d.split('.') {
        if label.is_empty() || label.len() > 63 {
            return false;
        }
        if label.starts_with('-') || label.ends_with('-') {
            return false;
        }
        if !label.chars().all(|c| c.is_ascii_alphanumeric() || c == '-') {
            return false;
        }
    }
    true
}

/// Checks if a string is a valid IP address or domain name
pub fn is_valid_host(host: &str) -> bool {
    let h = host.trim();
    if h.is_empty() {
        return false;
    }
    is_valid_ip(h) || is_valid_domain(h)
}

/// Validates host:port, [ipv6]:port, or domain:port
pub fn is_valid_host_port(hp: &str) -> bool {
    let s = hp.trim();
    if s.is_empty() {
        return false;
    }
    // [ipv6]:port
    if s.starts_with('[') {
        if let Some(close_bracket) = s.find(']') {
            let ip_str = &s[1..close_bracket];
            if !is_valid_ipv6(ip_str) {
                return false;
            }
            if !s[close_bracket + 1..].starts_with(':') {
                return false;
            }
            let port_str = &s[close_bracket + 2..];
            return port_str.parse::<u16>().map(|p| p > 0).unwrap_or(false);
        }
        return false;
    }
    // host:port (domain:port or ipv4:port)
    if let Some(colon) = s.rfind(':') {
        let host = &s[..colon];
        let port_str = &s[colon + 1..];
        if !is_valid_host(host) {
            return false;
        }
        return port_str.parse::<u16>().map(|p| p > 0).unwrap_or(false);
    }
    false
}

const WHITELISTED_LG_PREFIXES: &[&str] = &[
    "ping ",
    "traceroute ",
    "mtr ",
    "birdc show ",
    "ip route show",
    "ip route get ",
    "ip -4 route show",
    "ip -4 route get ",
    "ip -6 route show",
    "ip -6 route get ",
    "ip link show",
    "ip addr show",
    "whois ",
    "dig ",
    "tcp_ping ",
];

/// Validates that an ad-hoc Looking Glass command string is safe to execute
pub fn validate_adhoc_command(cmd: &str) -> Result<(), String> {
    let cmd = cmd.trim();
    if cmd.is_empty() {
        return Err("command cannot be empty".to_string());
    }

    // Must not contain shell metacharacters, quotes, or backslashes
    if cmd.chars().any(|c| matches!(c, ';' | '&' | '|' | '`' | '$' | '\n' | '\r' | '>' | '<' | '\'' | '"' | '\\')) {
        return Err("command contains dangerous shell metacharacters".to_string());
    }

    // Check whitelist prefixes
    let matched = WHITELISTED_LG_PREFIXES.iter().any(|prefix| {
        cmd.starts_with(prefix) || cmd == prefix.trim()
    });

    if !matched {
        return Err(format!(
            "command must start with one of: {}",
            WHITELISTED_LG_PREFIXES.join(", ")
        ));
    }

    let fields: Vec<&str> = cmd.split_whitespace().collect();
    if fields.is_empty() {
        return Err("command cannot be empty".to_string());
    }

    let prog = fields[0];
    match prog {
        "dig" => {
            // Disallow batch file flags (-f, -b, -k, -y)
            for arg in &fields[1..] {
                if arg.starts_with("-f") || arg.starts_with("-b") || arg.starts_with("-k") || arg.starts_with("-y") {
                    return Err(format!("flag '{}' is not allowed in dig", arg));
                }
            }
        }
        "ping" => {
            // Disallow flood ping (-f) and cap packet counts
            for (i, arg) in fields[1..].iter().enumerate() {
                if *arg == "-f" || arg.starts_with("-f") {
                    return Err("flood ping flag is not allowed".to_string());
                }
                if *arg == "-c" && i + 1 < fields[1..].len() {
                    if let Ok(count) = fields[1..][i + 1].parse::<u32>() {
                        if count > 30 {
                            return Err("ping count exceeds maximum allowed limit of 30".to_string());
                        }
                    }
                }
            }
        }
        "ip" => {
            // Ensure only read-only subcommands
            for arg in &fields[1..] {
                match arg.to_lowercase().as_str() {
                    "del" | "delete" | "add" | "flush" | "replace" | "change" | "set" => {
                        return Err(format!("subcommand '{}' is not permitted in ip", arg));
                    }
                    _ => {}
                }
            }
        }
        "birdc" => {
            if fields.len() < 2 || fields[1] != "show" {
                return Err("only 'birdc show' subcommands are permitted".to_string());
            }
        }
        _ => {}
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_valid_interface_names() {
        assert!(is_valid_iface_name("wg42_node1"));
        assert!(is_valid_iface_name("eth0"));
        assert!(is_valid_iface_name("wg-0"));
        assert!(!is_valid_iface_name(""));
        assert!(!is_valid_iface_name("wg42_super_long_interface_name"));
        assert!(!is_valid_iface_name("wg0;rm -rf /"));
    }

    #[test]
    fn test_valid_node_names() {
        assert!(is_valid_node_name("fra-node-01"));
        assert!(is_valid_node_name("node_2"));
        assert!(!is_valid_node_name(""));
        assert!(!is_valid_node_name("node 1"));
        assert!(!is_valid_node_name("node$bad"));
    }

    #[test]
    fn test_valid_ip() {
        assert!(is_valid_ipv4("10.0.0.1"));
        assert!(!is_valid_ipv4("999.999.999.999"));
        assert!(is_valid_ipv6("fe80::1"));
        assert!(is_valid_ipv6("fda2:200:1::1/64"));
        assert!(is_valid_ip("192.168.1.1"));
        assert!(is_valid_ip("2001:db8::1"));
        assert!(!is_valid_ip("invalid-ip"));
    }

    #[test]
    fn test_valid_domain_and_host() {
        assert!(is_valid_domain("example.com"));
        assert!(is_valid_domain("sub.domain.dn42"));
        assert!(!is_valid_domain("-bad.domain"));
        assert!(!is_valid_domain("bad..domain"));
        assert!(is_valid_host("1.1.1.1"));
        assert!(is_valid_host("google.com"));
        assert!(!is_valid_host("bad host; rm"));
    }

    #[test]
    fn test_valid_host_port() {
        assert!(is_valid_host_port("1.1.1.1:53"));
        assert!(is_valid_host_port("google.com:443"));
        assert!(is_valid_host_port("[2001:db8::1]:80"));
        assert!(!is_valid_host_port("google.com:bad"));
        assert!(!is_valid_host_port("google.com:99999"));
        assert!(!is_valid_host_port("[2001:db8::1]"));
    }

    #[test]
    fn test_validate_adhoc_command() {
        assert!(validate_adhoc_command("ping -c 4 1.1.1.1").is_ok());
        assert!(validate_adhoc_command("birdc show route").is_ok());
        assert!(validate_adhoc_command("ip -6 route show").is_ok());
        assert!(validate_adhoc_command("ip route get 1.1.1.1").is_ok());
        assert!(validate_adhoc_command("dig google.com").is_ok());

        // Attacks
        assert!(validate_adhoc_command("ping -f 1.1.1.1").is_err());
        assert!(validate_adhoc_command("dig -f /etc/shadow").is_err());
        assert!(validate_adhoc_command("ip route del default").is_err());
        assert!(validate_adhoc_command("birdc reload").is_err());
        assert!(validate_adhoc_command("cat /etc/passwd").is_err());
        assert!(validate_adhoc_command("ping 1.1.1.1; id").is_err());
    }
}
