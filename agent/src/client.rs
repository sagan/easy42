use futures_util::{SinkExt, StreamExt};
use http::Request;
use prost::Message;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::Mutex;
use tokio::time::{interval, sleep, timeout};
use tokio_tungstenite::tungstenite::Message as WsMessage;
use tracing::{error, info, warn};

use crate::collector::{proto, MetricsCollector};
use crate::config::AgentConfig;
use crate::executor::CommandExecutor;

pub struct AgentClient {
    config: AgentConfig,
    executor: Arc<CommandExecutor>,
    collector: Arc<Mutex<MetricsCollector>>,
}

impl AgentClient {
    pub fn new(config: AgentConfig) -> Self {
        Self {
            config,
            executor: Arc::new(CommandExecutor::new()),
            collector: Arc::new(Mutex::new(MetricsCollector::new())),
        }
    }

    pub async fn run(&self) {
        let mut backoff = Duration::from_secs(2);
        let max_backoff = Duration::from_secs(60);

        loop {
            info!(
                "Connecting to Easy42 server at {}...",
                self.config.server_ws_url()
            );

            let start = std::time::Instant::now();
            match self.connect_and_serve().await {
                Ok(()) => {
                    info!("Connection closed normally, reconnecting...");
                    backoff = Duration::from_secs(2);
                }
                Err(e) => {
                    if start.elapsed() > Duration::from_secs(30) {
                        backoff = Duration::from_secs(2);
                    }
                    error!("Connection error: {}. Retrying in {:?}...", e, backoff);
                    sleep(backoff).await;
                    backoff = (backoff * 2).min(max_backoff);
                }
            }
        }
    }

    async fn connect_and_serve(&self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let ws_url = self.config.server_ws_url();

        let request = Request::builder()
            .uri(&ws_url)
            .header("Authorization", format!("Bearer {}", self.config.token))
            .header("X-Easy42-Agent-Token", &self.config.token)
            .header("User-Agent", "easy42-agent/0.1.0")
            .header("Host", get_host_from_url(&ws_url))
            .header("Connection", "Upgrade")
            .header("Upgrade", "websocket")
            .header("Sec-WebSocket-Version", "13")
            .header(
                "Sec-WebSocket-Key",
                tokio_tungstenite::tungstenite::handshake::client::generate_key(),
            )
            .body(())?;

        // 1. Connect with a 10-second timeout to prevent hanging indefinitely on stalls
        let connect_timeout = Duration::from_secs(10);
        let (ws_stream, _) = match timeout(connect_timeout, tokio_tungstenite::connect_async(request)).await {
            Ok(res) => res?,
            Err(_) => {
                return Err(format!("Connection attempt timed out after {:?}", connect_timeout).into());
            }
        };
        info!("WebSocket connected successfully to {}", ws_url);

        let (mut ws_sender, mut ws_receiver) = ws_stream.split();

        // 2. Send RegisterRequest with timeout
        let hostname = get_system_hostname();
        let os_info = format!("{} {}", std::env::consts::OS, std::env::consts::ARCH);
        let node_name = self
            .config
            .node_name
            .clone()
            .unwrap_or_else(|| hostname.clone());

        let reg_msg = proto::AgentMessage {
            seq: 1,
            timestamp: now_millis(),
            payload: Some(proto::agent_message::Payload::RegisterReq(
                proto::RegisterRequest {
                    node_name,
                    agent_version: "0.1.0".to_string(),
                    os_info,
                    hostname,
                },
            )),
        };

        let mut buf = Vec::new();
        reg_msg.encode(&mut buf)?;
        match timeout(Duration::from_secs(10), ws_sender.send(WsMessage::Binary(buf.into()))).await {
            Ok(res) => res?,
            Err(_) => {
                return Err("Timed out sending registration request".into());
            }
        };

        // Channels for outgoing messages from tickers and pong responses
        enum OutgoingMessage {
            Proto(proto::AgentMessage),
            Raw(WsMessage),
        }

        let (tx, mut rx) = tokio::sync::mpsc::channel::<OutgoingMessage>(32);

        // Task: Telemetry ticker
        let tx_telemetry = tx.clone();
        let collector_clone = self.collector.clone();
        let interval_secs = self.config.telemetry_interval_secs;
        let telemetry_task = tokio::spawn(async move {
            let mut ticker = interval(Duration::from_secs(interval_secs));
            loop {
                ticker.tick().await;
                let telemetry = {
                    let mut col = collector_clone.lock().await;
                    col.collect_telemetry().await
                };

                let msg = proto::AgentMessage {
                    seq: 0,
                    timestamp: now_millis(),
                    payload: Some(proto::agent_message::Payload::Telemetry(telemetry)),
                };
                if tx_telemetry.send(OutgoingMessage::Proto(msg)).await.is_err() {
                    break;
                }
            }
        });

        // Task: Heartbeat ticker (every 15 seconds)
        let tx_hb = tx.clone();
        let heartbeat_task = tokio::spawn(async move {
            let mut ticker = interval(Duration::from_secs(15));
            loop {
                ticker.tick().await;
                let msg = proto::AgentMessage {
                    seq: 0,
                    timestamp: now_millis(),
                    payload: Some(proto::agent_message::Payload::Heartbeat(proto::Heartbeat {
                        client_time: now_millis(),
                    })),
                };
                if tx_hb.send(OutgoingMessage::Proto(msg)).await.is_err() {
                    break;
                }
            }
        });

        // Writer future forwarding mpsc to WebSocket sink with per-send timeout
        let writer_fut = async {
            while let Some(out_msg) = rx.recv().await {
                let ws_msg = match out_msg {
                    OutgoingMessage::Proto(msg) => {
                        let mut buf = Vec::new();
                        if msg.encode(&mut buf).is_err() {
                            continue;
                        }
                        WsMessage::Binary(buf.into())
                    }
                    OutgoingMessage::Raw(raw) => raw,
                };

                match timeout(Duration::from_secs(10), ws_sender.send(ws_msg)).await {
                    Ok(Ok(())) => {}
                    Ok(Err(e)) => return Err(format!("WebSocket write error: {}", e)),
                    Err(_) => return Err("WebSocket write timed out (10s)".to_string()),
                }
            }
            Ok::<(), String>(())
        };

        // Reader future processing incoming server messages with read timeout (45s)
        let executor = self.executor.clone();
        let tx_cmd_resp = tx.clone();
        let tx_pong = tx.clone();
        let reader_fut = async {
            loop {
                let msg_result = match timeout(Duration::from_secs(45), ws_receiver.next()).await {
                    Ok(Some(res)) => res,
                    Ok(None) => {
                        info!("WebSocket stream closed by server");
                        return Ok(());
                    }
                    Err(_) => {
                        return Err("WebSocket read timed out (no data from server for 45s)".to_string());
                    }
                };

                match msg_result {
                    Ok(WsMessage::Binary(bin)) => {
                        if let Ok(agent_msg) = proto::AgentMessage::decode(&bin[..]) {
                            match agent_msg.payload {
                                Some(proto::agent_message::Payload::CommandReq(cmd)) => {
                                    info!("Received command request: id={}", cmd.request_id);
                                    let exec = executor.clone();
                                    let is_flush = matches!(
                                        cmd.command,
                                        Some(proto::command_request::Command::FlushCache(_))
                                    );
                                    let is_probe = matches!(
                                        cmd.command,
                                        Some(proto::command_request::Command::ProbeSystem(_))
                                    );
                                    let col_opt = if is_probe || is_flush {
                                        Some(self.collector.clone())
                                    } else {
                                        None
                                    };
                                    let resp_tx = tx_cmd_resp.clone();
                                    tokio::spawn(async move {
                                        if let Some(col) = col_opt {
                                            let telemetry = {
                                                let mut c = col.lock().await;
                                                if is_flush || is_probe {
                                                    c.flush_interface_cache();
                                                }
                                                c.collect_telemetry().await
                                            };
                                            let telem_msg = proto::AgentMessage {
                                                seq: 0,
                                                timestamp: now_millis(),
                                                payload: Some(
                                                    proto::agent_message::Payload::Telemetry(telemetry),
                                                ),
                                            };
                                            let _ = resp_tx.send(OutgoingMessage::Proto(telem_msg)).await;
                                        }
                                        let resp = exec.execute(cmd).await;
                                        let out_msg = proto::AgentMessage {
                                            seq: 0,
                                            timestamp: now_millis(),
                                            payload: Some(proto::agent_message::Payload::CommandResp(
                                                resp,
                                            )),
                                        };
                                        let _ = resp_tx.send(OutgoingMessage::Proto(out_msg)).await;
                                    });
                                }
                                Some(proto::agent_message::Payload::HeartbeatAck(_)) => {
                                    // Heartbeat acknowledged
                                }
                                Some(proto::agent_message::Payload::RegisterResp(r)) => {
                                    if r.success {
                                        info!("Agent successfully registered with Easy42 controller");
                                    } else {
                                        warn!("Registration failed: {}", r.error_message);
                                        return Err(format!("Registration rejected by server: {}", r.error_message));
                                    }
                                }
                                _ => {}
                            }
                        }
                    }
                    Ok(WsMessage::Ping(payload)) => {
                        let _ = tx_pong.send(OutgoingMessage::Raw(WsMessage::Pong(payload))).await;
                    }
                    Ok(WsMessage::Close(_)) => {
                        info!("Received WebSocket Close frame");
                        return Ok(());
                    }
                    Err(e) => {
                        return Err(format!("WebSocket read error: {}", e));
                    }
                    _ => {}
                }
            }
        };

        let result = tokio::select! {
            r = writer_fut => match r {
                Ok(()) => Ok(()),
                Err(e) => Err(e.into()),
            },
            r = reader_fut => match r {
                Ok(()) => Ok(()),
                Err(e) => Err(e.into()),
            },
        };

        telemetry_task.abort();
        heartbeat_task.abort();

        result
    }
}

fn now_millis() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

fn get_system_hostname() -> String {
    std::fs::read_to_string("/proc/sys/kernel/hostname")
        .map(|s| s.trim().to_string())
        .unwrap_or_else(|_| "unknown".to_string())
}

fn get_host_from_url(url: &str) -> String {
    let without_proto = url
        .trim_start_matches("wss://")
        .trim_start_matches("ws://")
        .trim_start_matches("https://")
        .trim_start_matches("http://");
    without_proto
        .split('/')
        .next()
        .unwrap_or("localhost")
        .to_string()
}
