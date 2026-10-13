use std::sync::Arc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};
use tokio::sync::mpsc;
use tokio::sync::Mutex;
use tokio::time::sleep;
use tracing::{error, info, warn};

use crate::collector::proto;
use crate::executor::{run_cmd, run_tcp_ping};

/// Howard Hinnant algorithm to convert days since 1970-01-01 to (year, month 1..12, day 1..31)
fn civil_from_days(days: i64) -> (i32, u32, u32) {
    let z = days + 719468;
    let era = (if z >= 0 { z } else { z - 146096 }) / 146097;
    let doe = (z - era * 146097) as u32;
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    let y = yoe as i32 + era as i32 * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let y = if m <= 2 { y + 1 } else { y };
    (y, m, d)
}

/// Convert unix epoch seconds to (sec, min, hour, dom, mon, dow)
/// dow: 0 = Sunday, 1 = Monday, ..., 6 = Saturday
fn break_down_timestamp(ts: i64) -> (u32, u32, u32, u32, u32, u32) {
    let sec = (ts % 60) as u32;
    let min = ((ts / 60) % 60) as u32;
    let hour = ((ts / 3600) % 24) as u32;
    let days = ts / 86400;
    let dow = (((days + 4) % 7 + 7) % 7) as u32; // Jan 1 1970 was Thursday (4)
    let (_, mon, dom) = civil_from_days(days);
    (sec, min, hour, dom, mon, dow)
}

#[derive(Clone, Debug)]
pub struct CronExpression {
    pub has_seconds: bool,
    pub sec: [bool; 60],
    pub min: [bool; 60],
    pub hour: [bool; 24],
    pub dom: [bool; 32],
    pub mon: [bool; 13],
    pub dow: [bool; 7],
}

impl CronExpression {
    pub fn parse(expr: &str) -> Result<Self, String> {
        let fields: Vec<&str> = expr.split_whitespace().collect();
        if fields.len() != 5 && fields.len() != 6 {
            return Err(format!(
                "expected 5 or 6 fields in cron expression, got {}",
                fields.len()
            ));
        }

        let has_seconds = fields.len() == 6;
        let mut sec = [false; 60];
        let mut min = [false; 60];
        let mut hour = [false; 24];
        let mut dom = [false; 32];
        let mut mon = [false; 13];
        let mut dow = [false; 7];

        let offset = if has_seconds { 1 } else { 0 };

        if has_seconds {
            parse_field(fields[0], 0, 59, &mut sec)?;
        } else {
            sec[0] = true; // default 0 second for 5-field cron
        }

        parse_field(fields[offset], 0, 59, &mut min)?;
        parse_field(fields[offset + 1], 0, 23, &mut hour)?;
        parse_field(fields[offset + 2], 1, 31, &mut dom)?;
        parse_field(fields[offset + 3], 1, 12, &mut mon)?;
        parse_field_dow(fields[offset + 4], &mut dow)?;

        Ok(Self {
            has_seconds,
            sec,
            min,
            hour,
            dom,
            mon,
            dow,
        })
    }

    pub fn matches(&self, sec: u32, min: u32, hour: u32, dom: u32, mon: u32, dow: u32) -> bool {
        if sec as usize >= 60
            || min as usize >= 60
            || hour as usize >= 24
            || dom as usize >= 32
            || mon as usize >= 13
            || dow as usize >= 7
        {
            return false;
        }

        self.sec[sec as usize]
            && self.min[min as usize]
            && self.hour[hour as usize]
            && self.dom[dom as usize]
            && self.mon[mon as usize]
            && self.dow[dow as usize]
    }

    /// Finds the next matching unix timestamp strictly greater than `after_ts`.
    /// Scans up to 366 days into the future.
    pub fn next_after(&self, after_ts: i64) -> Option<i64> {
        let max_scan = 366 * 86400;
        let start = after_ts + 1;
        let end = after_ts + max_scan;

        if self.has_seconds {
            // Second-by-second scan
            let mut curr = start;
            while curr <= end {
                let (s, m, h, d, mon, w) = break_down_timestamp(curr);
                if self.matches(s, m, h, d, mon, w) {
                    return Some(curr);
                }
                curr += 1;
            }
        } else {
            // Minute-by-minute scan
            // Align to current or next whole minute
            let mut curr = if start % 60 == 0 {
                start
            } else {
                start + (60 - (start % 60))
            };
            while curr <= end {
                let (s, m, h, d, mon, w) = break_down_timestamp(curr);
                if self.matches(s, m, h, d, mon, w) {
                    return Some(curr);
                }
                curr += 60;
            }
        }

        None
    }
}

fn parse_field(field: &str, min: u32, max: u32, target: &mut [bool]) -> Result<(), String> {
    if field == "*" || field == "?" {
        for i in min..=max {
            target[i as usize] = true;
        }
        return Ok(());
    }

    for part in field.split(',') {
        let part = part.trim();
        if part.is_empty() {
            continue;
        }

        if let Some((base, step_str)) = part.split_once('/') {
            let step: u32 = step_str
                .parse()
                .map_err(|_| format!("invalid step: {}", step_str))?;
            if step == 0 {
                return Err("step cannot be 0".to_string());
            }

            let (start, end) = if base == "*" || base.is_empty() {
                (min, max)
            } else if let Some((s_str, e_str)) = base.split_once('-') {
                let s: u32 = s_str.parse().map_err(|_| "invalid range start")?;
                let e: u32 = e_str.parse().map_err(|_| "invalid range end")?;
                (s.max(min), e.min(max))
            } else {
                let s: u32 = base.parse().map_err(|_| "invalid start")?;
                (s.max(min), max)
            };

            let mut val = start;
            while val <= end {
                target[val as usize] = true;
                val += step;
            }
        } else if let Some((s_str, e_str)) = part.split_once('-') {
            let s: u32 = s_str.parse().map_err(|_| "invalid range start")?;
            let e: u32 = e_str.parse().map_err(|_| "invalid range end")?;
            for val in s.max(min)..=e.min(max) {
                target[val as usize] = true;
            }
        } else {
            let val: u32 = part.parse().map_err(|_| format!("invalid value: {}", part))?;
            if val < min || val > max {
                return Err(format!("value {} out of bounds [{}, {}]", val, min, max));
            }
            target[val as usize] = true;
        }
    }

    Ok(())
}

fn parse_field_dow(field: &str, target: &mut [bool; 7]) -> Result<(), String> {
    if field == "*" || field == "?" {
        for i in 0..7 {
            target[i] = true;
        }
        return Ok(());
    }

    let mut raw = [false; 8]; // 0..7 (7 is also Sunday in standard cron)
    parse_field(field, 0, 7, &mut raw)?;
    for i in 0..7 {
        target[i] = raw[i];
    }
    if raw[7] {
        target[0] = true; // 7 -> 0 (Sunday)
    }
    Ok(())
}

#[derive(Clone, Debug)]
pub struct ScheduledCronTask {
    pub task_id: String,
    pub name: String,
    pub task_type: String,
    #[allow(dead_code)]
    pub cron_expr_str: String,
    pub target: String,
    pub count: u32,
    pub timeout_seconds: u32,
    pub parsed_cron: CronExpression,
    pub next_run_secs: i64,
}

pub struct CronScheduler {
    node_name: String,
    tasks: Arc<Mutex<Vec<ScheduledCronTask>>>,
}

impl CronScheduler {
    pub fn new(node_name: String) -> Self {
        Self {
            node_name,
            tasks: Arc::new(Mutex::new(Vec::new())),
        }
    }

    /// Replaces the current active cron tasks in memory with new specs pushed by the server
    pub async fn update_tasks(&self, specs: Vec<proto::CronTaskSpec>) {
        let now = now_secs();
        let mut new_tasks = Vec::new();

        for spec in specs {
            match CronExpression::parse(&spec.cron_expr) {
                Ok(cron) => {
                    let next_run = cron.next_after(now).unwrap_or(now + 60);
                    info!(
                        "Scheduled cron task '{}' ({}) next run in {}s",
                        spec.name,
                        spec.task_id,
                        (next_run - now).max(0)
                    );
                    new_tasks.push(ScheduledCronTask {
                        task_id: spec.task_id,
                        name: spec.name,
                        task_type: spec.task_type,
                        cron_expr_str: spec.cron_expr,
                        target: spec.target,
                        count: if spec.count > 0 { spec.count } else { 4 },
                        timeout_seconds: if spec.timeout_seconds > 0 {
                            spec.timeout_seconds
                        } else {
                            15
                        },
                        parsed_cron: cron,
                        next_run_secs: next_run,
                    });
                }
                Err(e) => {
                    error!(
                        "Failed to parse cron expression '{}' for task '{}': {}",
                        spec.cron_expr, spec.name, e
                    );
                }
            }
        }

        let mut lock = self.tasks.lock().await;
        *lock = new_tasks;
        info!(
            "Cron tasks synchronized for node '{}': {} task(s) active in memory",
            self.node_name,
            lock.len()
        );
    }

    /// Background runner that ticks every 1 second and executes due cron tasks
    pub async fn run_loop(&self, tx: mpsc::Sender<proto::AgentMessage>) {
        info!("Cron task scheduler loop started for node '{}'", self.node_name);

        loop {
            sleep(Duration::from_secs(1)).await;
            let now = now_secs();

            let mut due_tasks = Vec::new();
            {
                let mut lock = self.tasks.lock().await;
                for task in lock.iter_mut() {
                    if now >= task.next_run_secs {
                        due_tasks.push(task.clone());
                        // Compute next run time
                        task.next_run_secs = task
                            .parsed_cron
                            .next_after(now)
                            .unwrap_or(now + 60);
                    }
                }
            }

            for task in due_tasks {
                let tx_clone = tx.clone();
                let node_name = self.node_name.clone();

                tokio::spawn(async move {
                    Self::execute_task(task, node_name, tx_clone).await;
                });
            }
        }
    }

    async fn execute_task(
        task: ScheduledCronTask,
        node_name: String,
        tx: mpsc::Sender<proto::AgentMessage>,
    ) {
        let start = Instant::now();
        let executed_at = now_secs();

        info!(
            "Executing cron task '{}' ({}) of type '{}' target='{}'",
            task.name, task.task_id, task.task_type, task.target
        );

        let (success, exit_code, stdout, stderr) = match task.task_type.as_str() {
            "tcp_ping" => {
                run_tcp_ping(&task.target, task.count, task.timeout_seconds).await
            }
            _ => {
                // Default: ping
                let count_str = task.count.to_string();
                let timeout_str = task.timeout_seconds.to_string();
                run_cmd("ping", &["-c", &count_str, "-W", &timeout_str, &task.target]).await
            }
        };

        let duration_ms = start.elapsed().as_millis() as u64;
        let raw_output = if stdout.is_empty() {
            stderr.clone()
        } else if !stderr.is_empty() {
            format!("{}\n{}", stdout, stderr)
        } else {
            stdout
        };

        let error_message = if !success && !stderr.is_empty() {
            stderr
        } else if !success {
            format!("exited with code {}", exit_code)
        } else {
            String::new()
        };

        let report = proto::CronTaskReport {
            task_id: task.task_id,
            node_name,
            executed_at,
            exit_code,
            duration_ms,
            raw_output,
            error_message,
        };

        let out_msg = proto::AgentMessage {
            seq: 0,
            timestamp: now_millis(),
            payload: Some(proto::agent_message::Payload::CronTaskReport(report)),
        };

        if let Err(e) = tx.send(out_msg).await {
            warn!("Failed to send cron task report to server: {}", e);
        }
    }
}

fn now_secs() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

fn now_millis() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis().min(i64::MAX as u128) as i64)
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_cron_parsing_and_matching() {
        let cron = CronExpression::parse("0 */1 * * *").expect("parse 5 fields");
        assert!(!cron.has_seconds);
        assert!(cron.min[0]);
        assert!(!cron.min[1]);
        assert!(cron.matches(0, 0, 12, 1, 1, 0));
        assert!(!cron.matches(0, 5, 12, 1, 1, 0));

        let cron6 = CronExpression::parse("*/10 * * * * *").expect("parse 6 fields");
        assert!(cron6.has_seconds);
        assert!(cron6.sec[0]);
        assert!(cron6.sec[10]);
        assert!(!cron6.sec[5]);
    }

    #[test]
    fn test_cron_next_after() {
        let cron = CronExpression::parse("*/5 * * * *").expect("parse");
        let ts = 1700000000; // arbitrary unix timestamp
        let next = cron.next_after(ts).expect("find next");
        assert!(next > ts);
        let (_, min, _, _, _, _) = break_down_timestamp(next);
        assert_eq!(min % 5, 0);
    }
}
