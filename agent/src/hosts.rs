use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use sha2::{Digest, Sha256};
use crate::proto::HostRecord;

pub const HOSTS_PATH: &str = "/etc/hosts";
#[allow(dead_code)]
pub const HOSTS_TMP_PATH: &str = "/etc/hosts-easy42-tmp";
pub const HOSTS_START_MARKER: &str = "### EASY42_START";
pub const HOSTS_END_MARKER: &str = "### EASY42_END";

pub fn get_hosts_path() -> PathBuf {
    if let Ok(path) = std::env::var("EASY42_HOSTS_PATH") {
        if !path.trim().is_empty() {
            return PathBuf::from(path.trim());
        }
    }
    PathBuf::from(HOSTS_PATH)
}

pub fn get_hosts_tmp_path() -> PathBuf {
    let hosts_path = get_hosts_path();
    let file_name = hosts_path
        .file_name()
        .and_then(|n| n.to_str())
        .unwrap_or("hosts");
    hosts_path.with_file_name(format!("{}-easy42-tmp", file_name))
}

/// Assembles the easy42 hosts block from records, sorted by node name ascending.
pub fn build_hosts_block(mut records: Vec<HostRecord>) -> String {
    records.sort_by(|a, b| a.name.cmp(&b.name));
    let mut out = String::new();
    out.push_str(HOSTS_START_MARKER);
    out.push('\n');
    for r in records {
        let ip = r.ip.trim();
        let name = r.name.trim();
        if !ip.is_empty()
            && !name.is_empty()
            && crate::validation::is_valid_ip(ip)
            && crate::validation::is_valid_node_name(name)
        {
            out.push_str(&format!("{} {} {}.internal\n", ip, name, name));
        }
    }
    out.push_str(HOSTS_END_MARKER);
    out.push('\n');
    out
}

/// Computes the SHA-256 hash of the normalized hosts block content.
pub fn hash_hosts_block(block: &str) -> String {
    let norm = block.replace("\r\n", "\n");
    let trimmed = norm.trim();
    if trimmed.is_empty() {
        return String::new();
    }
    let mut hasher = Sha256::new();
    hasher.update(trimmed.as_bytes());
    format!("{:x}", hasher.finalize())
}

/// Extracts the easy42 block and its SHA-256 hash from hosts file content.
pub fn extract_hosts_block(content: &str) -> Option<(String, String)> {
    let norm = content.replace("\r\n", "\n");
    let start_idx = norm.find(HOSTS_START_MARKER)?;
    let end_relative = norm[start_idx..].find(HOSTS_END_MARKER)?;
    let end_idx = start_idx + end_relative + HOSTS_END_MARKER.len();
    let block = &norm[start_idx..end_idx];
    let hash = hash_hosts_block(block);
    Some((block.to_string(), hash))
}

/// Reads current /etc/hosts file and returns the SHA-256 hash of its easy42 block, or empty string.
pub fn get_current_hosts_hash(hosts_path: &Path) -> String {
    if let Ok(content) = fs::read_to_string(hosts_path) {
        if let Some((_, hash)) = extract_hosts_block(&content) {
            return hash;
        }
    }
    String::new()
}

/// Updates the hosts file in an atomic way:
/// 1. Read existing /etc/hosts file to memory
/// 2. Check if easy42 block need to be updated and update it if necessary
/// 3. Write the updated contents to temp file (/etc/hosts-easy42-tmp)
/// 4. Atomically rename /etc/hosts-easy42-tmp to /etc/hosts
/// Returns Ok((updated, new_hash)) or Err(error_message).
pub fn update_hosts_file(
    hosts_path: &Path,
    tmp_path: &Path,
    records: Vec<HostRecord>,
) -> Result<(bool, String), String> {
    // 1. Read existing /etc/hosts file to memory
    let existing_content = match fs::read_to_string(hosts_path) {
        Ok(s) => s,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => String::new(),
        Err(e) => return Err(format!("failed to read {:?}: {}", hosts_path, e)),
    };

    let new_block = build_hosts_block(records);
    let new_hash = hash_hosts_block(&new_block);

    // Assemble new content preserving all other lines, ensuring at least 1 new line (\n)
    // exists before and after the easy42 block.
    let norm_existing = existing_content.replace("\r\n", "\n");
    let (prefix, suffix) = if let Some(start_idx) = norm_existing.find(HOSTS_START_MARKER) {
        let pre = &norm_existing[..start_idx];
        let suf = if let Some(end_rel) = norm_existing[start_idx..].find(HOSTS_END_MARKER) {
            let end_idx = start_idx + end_rel + HOSTS_END_MARKER.len();
            // Skip the single newline immediately following ### EASY42_END if present
            if norm_existing[end_idx..].starts_with('\n') {
                &norm_existing[end_idx + 1..]
            } else {
                &norm_existing[end_idx..]
            }
        } else {
            ""
        };
        (pre, suf)
    } else {
        (norm_existing.as_str(), "")
    };

    let prefix_str = if prefix.is_empty() {
        "\n".to_string()
    } else if prefix.trim().is_empty() {
        if prefix.contains('\n') {
            prefix.to_string()
        } else {
            "\n".to_string()
        }
    } else if prefix.ends_with("\n\n") {
        prefix.to_string()
    } else if prefix.ends_with('\n') {
        format!("{}\n", prefix)
    } else {
        format!("{}\n\n", prefix)
    };

    let suffix_str = if suffix.is_empty() {
        "\n".to_string()
    } else if suffix.starts_with('\n') {
        suffix.to_string()
    } else {
        format!("\n{}", suffix)
    };

    let new_content = format!("{}{}{}", prefix_str, new_block, suffix_str);

    // 2. Check if file content (block and surrounding newlines) is already up-to-date
    if new_content == norm_existing {
        return Ok((false, new_hash));
    }

    // 3. Write the updated contents to temp file
    {
        let mut tmp_file = fs::OpenOptions::new()
            .create(true)
            .write(true)
            .truncate(true)
            .open(tmp_path)
            .map_err(|e| format!("failed to create temp file {:?}: {}", tmp_path, e))?;

        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let _ = fs::set_permissions(tmp_path, fs::Permissions::from_mode(0o644));
        }

        tmp_file
            .write_all(new_content.as_bytes())
            .map_err(|e| format!("failed to write to temp file {:?}: {}", tmp_path, e))?;

        tmp_file
            .sync_all()
            .map_err(|e| format!("failed to sync temp file {:?}: {}", tmp_path, e))?;
    }

    // 4. Atomically rename temp file to target hosts file
    fs::rename(tmp_path, hosts_path)
        .map_err(|e| format!("failed to rename {:?} to {:?}: {}", tmp_path, hosts_path, e))?;

    Ok((true, new_hash))
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    #[test]
    fn test_build_and_hash_hosts_block() {
        let records = vec![
            HostRecord {
                name: "foo".to_string(),
                ip: "192.168.100.1".to_string(),
            },
            HostRecord {
                name: "bar".to_string(),
                ip: "192.168.100.2".to_string(),
            },
        ];

        let block = build_hosts_block(records);
        let expected = "### EASY42_START\n192.168.100.2 bar bar.internal\n192.168.100.1 foo foo.internal\n### EASY42_END\n";
        assert_eq!(block, expected);

        let hash = hash_hosts_block(&block);
        assert!(!hash.is_empty());

        let (extracted_block, extracted_hash) = extract_hosts_block(&block).unwrap();
        assert_eq!(extracted_hash, hash);
        assert!(extracted_block.contains("192.168.100.2 bar bar.internal"));
    }

    #[test]
    fn test_atomic_update_preserves_content() {
        let hosts_file = NamedTempFile::new().unwrap();
        let hosts_path = hosts_file.path().to_path_buf();
        let tmp_path = hosts_path.with_extension("tmp");

        let initial_content = "127.0.0.1 localhost\n::1 localhost ip6-localhost\n# Custom comment\n";
        fs::write(&hosts_path, initial_content).unwrap();

        let records = vec![
            HostRecord {
                name: "foo".to_string(),
                ip: "192.168.100.1".to_string(),
            },
            HostRecord {
                name: "bar".to_string(),
                ip: "192.168.100.2".to_string(),
            },
        ];

        // First update adds the block
        let (updated, hash1) = update_hosts_file(&hosts_path, &tmp_path, records.clone()).unwrap();
        assert!(updated);
        assert!(!hash1.is_empty());

        let after_first = fs::read_to_string(&hosts_path).unwrap();
        assert!(after_first.starts_with("127.0.0.1 localhost\n::1 localhost ip6-localhost\n# Custom comment\n\n### EASY42_START"));
        assert!(after_first.ends_with("### EASY42_END\n\n"));
        assert!(after_first.contains("### EASY42_START\n192.168.100.2 bar bar.internal\n192.168.100.1 foo foo.internal\n### EASY42_END"));

        // Second update with same records does NOT modify file
        let (updated_again, hash2) = update_hosts_file(&hosts_path, &tmp_path, records).unwrap();
        assert!(!updated_again);
        assert_eq!(hash1, hash2);

        // Third update with new record replaces the block
        let new_records = vec![
            HostRecord {
                name: "bar".to_string(),
                ip: "192.168.100.2".to_string(),
            },
            HostRecord {
                name: "baz".to_string(),
                ip: "192.168.100.3".to_string(),
            },
            HostRecord {
                name: "foo".to_string(),
                ip: "192.168.100.1".to_string(),
            },
        ];

        let (updated_third, hash3) = update_hosts_file(&hosts_path, &tmp_path, new_records).unwrap();
        assert!(updated_third);
        assert_ne!(hash1, hash3);

        let after_third = fs::read_to_string(&hosts_path).unwrap();
        assert!(after_third.starts_with("127.0.0.1 localhost\n::1 localhost ip6-localhost\n# Custom comment\n\n### EASY42_START"));
        assert!(after_third.ends_with("### EASY42_END\n\n"));
        assert!(after_third.contains("192.168.100.3 baz baz.internal"));
    }

    #[test]
    fn test_atomic_update_ensures_newlines_when_previously_missing() {
        let hosts_file = NamedTempFile::new().unwrap();
        let hosts_path = hosts_file.path().to_path_buf();
        let tmp_path = hosts_path.with_extension("tmp");

        // Existing file without empty line before ### EASY42_START and without newline after ### EASY42_END
        let initial_missing_newlines = "::1 localhost\nff02::2 ip6-allrouters\n### EASY42_START\n192.168.100.1 foo foo.internal\n### EASY42_END";
        fs::write(&hosts_path, initial_missing_newlines).unwrap();

        let records = vec![
            HostRecord {
                name: "foo".to_string(),
                ip: "192.168.100.1".to_string(),
            },
        ];

        // Should detect missing newlines and rewrite the file
        let (updated, hash) = update_hosts_file(&hosts_path, &tmp_path, records.clone()).unwrap();
        assert!(updated);
        assert!(!hash.is_empty());

        let after = fs::read_to_string(&hosts_path).unwrap();
        let expected = "::1 localhost\nff02::2 ip6-allrouters\n\n### EASY42_START\n192.168.100.1 foo foo.internal\n### EASY42_END\n\n";
        assert_eq!(after, expected);

        // Next update is a no-op
        let (updated_again, hash2) = update_hosts_file(&hosts_path, &tmp_path, records).unwrap();
        assert!(!updated_again);
        assert_eq!(hash, hash2);
    }
}
