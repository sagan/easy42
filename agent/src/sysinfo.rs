//! Host architecture and endianness detection.
//!
//! `std::env::consts::ARCH` is resolved at *compile* time, so a `mipsel` agent
//! binary reports `mips` on every MIPS host regardless of the hardware. That is
//! not good enough for artifact resolution, which needs to pick between
//! `mips-*` and `mipsel-*` upstream assets. We therefore query the running
//! kernel and fall back to the compile-time value only as a last resort.

use std::process::Command;

/// Returns the kernel-reported machine architecture (`uname -m`), normalized to
/// the names used by Rust target triples where the mapping is unambiguous.
///
/// MIPS is deliberately *not* normalized here: `uname -m` reports `mips` for
/// both endiannesses on many OpenWrt targets, so the caller must combine this
/// with [`endianness`].
pub fn machine_arch() -> String {
    let raw = uname_machine().unwrap_or_else(|| std::env::consts::ARCH.to_string());
    let lower = raw.trim().to_lowercase();

    match lower.as_str() {
        "amd64" | "x64" => "x86_64".to_string(),
        "arm64" | "arm64v8" => "aarch64".to_string(),
        "armv7l" | "armhf" => "armv7".to_string(),
        _ => lower,
    }
}

/// Returns the native endianness of the running CPU: `"little"` or `"big"`.
pub fn endianness() -> String {
    if cfg!(target_endian = "little") {
        "little".to_string()
    } else {
        "big".to_string()
    }
}

/// Builds the `os_info` string reported to the server: `<os> <arch> <endian>`.
///
/// Example: `linux mipsle little`. The third field is omitted for
/// architectures where endianness is not ambiguous, so existing consumers that
/// parse `os_info` keep seeing the familiar two-field form.
pub fn os_info() -> String {
    let arch = machine_arch();
    if arch.starts_with("mips") {
        format!("{} {} {}", std::env::consts::OS, arch, endianness())
    } else {
        format!("{} {}", std::env::consts::OS, arch)
    }
}

fn uname_machine() -> Option<String> {
    let output = Command::new("uname").arg("-m").output().ok()?;
    if !output.status.success() {
        return None;
    }
    let value = String::from_utf8_lossy(&output.stdout).trim().to_string();
    if value.is_empty() {
        None
    } else {
        Some(value)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn endianness_is_known() {
        let e = endianness();
        assert!(e == "little" || e == "big");
    }

    #[test]
    fn os_info_has_os_and_arch() {
        let info = os_info();
        let parts: Vec<&str> = info.split_whitespace().collect();
        assert!(parts.len() >= 2, "unexpected os_info: {info}");
        assert_eq!(parts[0], std::env::consts::OS);
    }

    #[test]
    fn os_info_reports_endianness_for_mips() {
        let arch = machine_arch();
        let info = os_info();
        let parts: Vec<&str> = info.split_whitespace().collect();
        if arch.starts_with("mips") {
            assert_eq!(parts.len(), 3, "MIPS os_info must carry endianness");
            assert!(parts[2] == "little" || parts[2] == "big");
        } else {
            assert_eq!(parts.len(), 2, "non-MIPS os_info stays two fields");
        }
    }
}
