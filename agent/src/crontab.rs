use sha2::{Digest, Sha256};
use tracing::{info, warn};
use crate::collector::proto::AppSpec;
use crate::executor::run_shell;

pub const CRON_START_MARKER: &str = "### EASY42_START";
pub const CRON_END_MARKER: &str = "### EASY42_END";

/// Derives a clean single cron line from a main_content string and enabled state.
/// If disabled, it comments the line with a '#' prefix (no space, e.g. #0 0 * * * /root/backup.sh).
/// If enabled, any leading '#' and whitespace are stripped.
pub fn format_cron_line(main_content: &str, enabled: bool) -> Option<String> {
    let trimmed = main_content.trim();
    if trimmed.is_empty() {
        return None;
    }

    if enabled {
        let clean = trimmed.trim_start_matches('#').trim();
        if clean.is_empty() {
            None
        } else {
            Some(clean.to_string())
        }
    } else {
        // Disabled: prefix with "#"
        if trimmed.starts_with('#') {
            let inner = trimmed.trim_start_matches('#').trim();
            Some(format!("#{}", inner))
        } else {
            Some(format!("#{}", trimmed))
        }
    }
}

/// Builds the EASY42 crontab block from apps.
/// All cron job lines are sorted by alphabetical order.
/// If there are no cron lines, returns an empty string.
pub fn build_cron_block_from_apps(apps: &[&AppSpec]) -> String {
    let mut lines = Vec::new();
    for app in apps {
        // If main_content has multiple lines, format each non-empty line
        for line in app.main_content.lines() {
            if let Some(formatted) = format_cron_line(line, app.enabled) {
                lines.push(formatted);
            }
        }
    }

    if lines.is_empty() {
        return String::new();
    }

    // "All cron job lines are sorted by alphabetical order."
    lines.sort();

    let mut out = String::new();
    out.push_str(CRON_START_MARKER);
    out.push('\n');
    for line in lines {
        out.push_str(&line);
        out.push('\n');
    }
    out.push_str(CRON_END_MARKER);
    out.push('\n');
    out
}

/// Extracts the EASY42 block from crontab content, if present.
pub fn extract_cron_block(content: &str) -> Option<String> {
    let norm = content.replace("\r\n", "\n");
    let start_idx = norm.find(CRON_START_MARKER)?;
    let end_rel = norm[start_idx..].find(CRON_END_MARKER)?;
    let end_idx = start_idx + end_rel + CRON_END_MARKER.len();
    Some(norm[start_idx..end_idx].to_string())
}

/// Computes SHA256 hash of a normalized cron block.
pub fn hash_cron_block(block: &str) -> String {
    let norm = block.replace("\r\n", "\n");
    let trimmed = norm.trim();
    if trimmed.is_empty() {
        return String::new();
    }
    let mut hasher = Sha256::new();
    hasher.update(trimmed.as_bytes());
    format!("{:x}", hasher.finalize())
}

/// Updates the crontab content by replacing or appending the EASY42 block.
/// Preserves all non-EASY42 lines.
/// Returns (changed, new_content).
pub fn update_crontab_content(existing_crontab: &str, desired_block: &str) -> (bool, String) {
    let norm_existing = existing_crontab.replace("\r\n", "\n");
    let existing_block_opt = extract_cron_block(&norm_existing);

    // If desired block is empty, we want to remove the EASY42 block
    if desired_block.trim().is_empty() {
        if existing_block_opt.is_none() {
            return (false, norm_existing);
        }
        // Remove block
        let (prefix, suffix) = split_around_block(&norm_existing);
        let mut result = String::new();
        let trimmed_prefix = prefix.trim_end();
        let trimmed_suffix = suffix.trim_start();
        if !trimmed_prefix.is_empty() {
            result.push_str(trimmed_prefix);
        }
        if !trimmed_suffix.is_empty() {
            if !result.is_empty() {
                result.push('\n');
            }
            result.push_str(trimmed_suffix);
        }
        if !result.is_empty() && !result.ends_with('\n') {
            result.push('\n');
        }
        return (true, result);
    }

    // Desired block is non-empty
    if let Some(existing_block) = existing_block_opt {
        if existing_block.trim() == desired_block.trim() {
            return (false, norm_existing);
        }
    }

    let (prefix, suffix) = split_around_block(&norm_existing);
    let mut result = String::new();
    let trimmed_prefix = prefix.trim_end();
    let trimmed_suffix = suffix.trim_start();

    if !trimmed_prefix.is_empty() {
        result.push_str(trimmed_prefix);
        result.push('\n');
        result.push('\n');
    }
    result.push_str(desired_block.trim());
    result.push('\n');

    if !trimmed_suffix.is_empty() {
        result.push('\n');
        result.push_str(trimmed_suffix);
    }
    if !result.ends_with('\n') {
        result.push('\n');
    }

    (true, result)
}

fn split_around_block(content: &str) -> (&str, &str) {
    if let Some(start_idx) = content.find(CRON_START_MARKER) {
        let prefix = &content[..start_idx];
        let suffix = if let Some(end_rel) = content[start_idx..].find(CRON_END_MARKER) {
            let end_idx = start_idx + end_rel + CRON_END_MARKER.len();
            if content[end_idx..].starts_with('\n') {
                &content[end_idx + 1..]
            } else {
                &content[end_idx..]
            }
        } else {
            ""
        };
        (prefix, suffix)
    } else {
        (content, "")
    }
}

/// Reads the current system crontab using standard `crontab -l`.
pub async fn read_system_crontab() -> Result<String, String> {
    let (success, code, stdout, stderr) = run_shell("crontab -l").await;
    if success {
        return Ok(stdout);
    }
    // "no crontab for <user>" is normal exit code 1
    let err_lower = stderr.to_lowercase();
    if code == 1 && (err_lower.contains("no crontab for") || stdout.trim().is_empty()) {
        return Ok(String::new());
    }
    Err(format!("failed to read crontab (code {}): {}", code, stderr))
}

/// Installs new crontab content using standard `crontab`.
pub async fn apply_crontab(new_content: &str) -> Result<(), String> {
    if new_content.trim().is_empty() {
        let (success, code, stdout, stderr) = run_shell("crontab -r 2>/dev/null || true").await;
        if !success && code != 0 {
            warn!("crontab -r returned code {}: {} {}", code, stdout, stderr);
        }
        return Ok(());
    }

    // Write to a temporary file, then run `crontab <temp_path>`
    let temp_dir = std::env::temp_dir();
    let temp_file_path = temp_dir.join(format!("easy42-cron-tmp-{}", std::process::id()));

    // Ensure content ends with newline (standard cron requirement)
    let content_to_write = if new_content.ends_with('\n') {
        new_content.to_string()
    } else {
        format!("{}\n", new_content)
    };

    std::fs::write(&temp_file_path, content_to_write.as_bytes())
        .map_err(|e| format!("failed to write temp cron file {:?}: {}", temp_file_path, e))?;

    let cmd = format!("crontab {}", crate::apps::shell_quote(temp_file_path.to_str().unwrap_or("")));
    let (success, code, stdout, stderr) = run_shell(&cmd).await;

    // Clean up temp file
    let _ = std::fs::remove_file(&temp_file_path);

    if !success {
        return Err(format!("crontab installation failed (code {}): {} {}", code, stdout, stderr));
    }

    Ok(())
}

/// Syncs the desired cron apps with system crontab.
/// "In the sync the agent receives the apps from the server, derive the desired crontab block contents
/// and compare it with system crontab existing easy42 block contents, if it's not matched then it use crontab to append or update easy42 block."
pub async fn sync_crontab_apps(cron_apps: &[&AppSpec]) -> Result<bool, String> {
    let desired_block = build_cron_block_from_apps(cron_apps);
    let existing_crontab = read_system_crontab().await?;

    let existing_block = extract_cron_block(&existing_crontab).unwrap_or_default();
    if existing_block.trim() == desired_block.trim() {
        info!("System crontab EASY42 block is already up-to-date");
        return Ok(false);
    }

    info!(
        "Updating system crontab EASY42 block (existing {} bytes, desired {} bytes)...",
        existing_block.len(),
        desired_block.len()
    );

    let (changed, new_crontab) = update_crontab_content(&existing_crontab, &desired_block);
    if changed {
        apply_crontab(&new_crontab).await?;
        info!("System crontab successfully updated");
        Ok(true)
    } else {
        Ok(false)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_format_cron_line() {
        assert_eq!(
            format_cron_line("0 0 * * * /root/backup.sh", true),
            Some("0 0 * * * /root/backup.sh".to_string())
        );

        assert_eq!(
            format_cron_line("0 0 * * * /root/backup.sh", false),
            Some("#0 0 * * * /root/backup.sh".to_string())
        );

        // Leading # when enabling should strip #
        assert_eq!(
            format_cron_line("#0 0 * * * /root/backup.sh", true),
            Some("0 0 * * * /root/backup.sh".to_string())
        );
        assert_eq!(
            format_cron_line("# 0 0 * * * /root/backup.sh", true),
            Some("0 0 * * * /root/backup.sh".to_string())
        );

        // When disabling an already commented line, ensure no space after #
        assert_eq!(
            format_cron_line("# 0 0 * * * /root/backup.sh", false),
            Some("#0 0 * * * /root/backup.sh".to_string())
        );
    }

    #[test]
    fn test_build_cron_block_sorted() {
        let app1 = AppSpec {
            id: "1".to_string(),
            name: "backup".to_string(),
            app_type: 2,
            main_content: "0 0 * * * /root/backup.sh".to_string(),
            env_content: String::new(),
            config_files: vec![],
            sha256_hash: String::new(),
            enabled: true,
        };
        let app2 = AppSpec {
            id: "2".to_string(),
            name: "clean".to_string(),
            app_type: 2,
            main_content: "30 2 * * * /root/clean.sh".to_string(),
            env_content: String::new(),
            config_files: vec![],
            sha256_hash: String::new(),
            enabled: false,
        };
        let app3 = AppSpec {
            id: "3".to_string(),
            name: "alpha".to_string(),
            app_type: 2,
            main_content: "10 * * * * /root/alpha.sh".to_string(),
            env_content: String::new(),
            config_files: vec![],
            sha256_hash: String::new(),
            enabled: true,
        };

        let block = build_cron_block_from_apps(&[&app1, &app2, &app3]);
        // # comes before numbers in ASCII (35 < 48)
        let expected = "### EASY42_START\n#30 2 * * * /root/clean.sh\n0 0 * * * /root/backup.sh\n10 * * * * /root/alpha.sh\n### EASY42_END\n";
        assert_eq!(block, expected);
    }

    #[test]
    fn test_extract_and_hash_block() {
        let block = "### EASY42_START\n0 0 * * * /root/backup.sh\n### EASY42_END\n";
        let extracted = extract_cron_block(block);
        assert_eq!(extracted, Some("### EASY42_START\n0 0 * * * /root/backup.sh\n### EASY42_END".to_string()));

        let h1 = hash_cron_block(block);
        assert!(!h1.is_empty());
    }

    #[test]
    fn test_update_crontab_content_append() {
        let existing = "# user custom cron\n0 5 * * * /usr/bin/custom.sh\n";
        let block = "### EASY42_START\n0 0 * * * /root/backup.sh\n### EASY42_END\n";
        let (changed, updated) = update_crontab_content(existing, block);
        assert!(changed);
        assert!(updated.contains("# user custom cron"));
        assert!(updated.contains("0 5 * * * /usr/bin/custom.sh"));
        assert!(updated.contains("### EASY42_START"));
        assert!(updated.contains("0 0 * * * /root/backup.sh"));
        assert!(updated.contains("### EASY42_END"));
        assert!(updated.ends_with('\n'));
    }

    #[test]
    fn test_update_crontab_content_replace() {
        let existing = "# header\n### EASY42_START\n0 0 * * * /root/old.sh\n### EASY42_END\n# footer\n";
        let new_block = "### EASY42_START\n0 0 * * * /root/new.sh\n### EASY42_END\n";
        let (changed, updated) = update_crontab_content(existing, new_block);
        assert!(changed);
        assert!(updated.contains("# header"));
        assert!(updated.contains("# footer"));
        assert!(updated.contains("0 0 * * * /root/new.sh"));
        assert!(!updated.contains("/root/old.sh"));
    }

    #[test]
    fn test_update_crontab_content_remove() {
        let existing = "# header\n\n### EASY42_START\n0 0 * * * /root/old.sh\n### EASY42_END\n\n# footer\n";
        let (changed, updated) = update_crontab_content(existing, "");
        assert!(changed);
        assert!(!updated.contains("### EASY42_START"));
        assert!(!updated.contains("/root/old.sh"));
        assert!(updated.contains("# header"));
        assert!(updated.contains("# footer"));
    }
}
