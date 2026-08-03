//! Minimal Codex app-server stdio adapter.
//!
//! Raw app-server messages stay inside this module. Callers receive normalized
//! thread metadata or bounded task identifiers, never stderr or prompt echoes.

use serde::Serialize;
use serde_json::json;
use serde_json::Value;
use std::collections::HashMap;
use std::fmt;
use std::io::BufRead;
use std::io::BufReader;
use std::io::Write;
use std::process::Child;
use std::process::ChildStdin;
use std::process::Command;
use std::process::Stdio;
use std::sync::atomic::AtomicU64;
use std::sync::atomic::Ordering;
use std::sync::mpsc;
use std::sync::Arc;
use std::sync::Mutex;
use std::thread;
use std::time::Duration;

const DEFAULT_REQUEST_TIMEOUT: Duration = Duration::from_secs(10);
const MAX_PROMPT_BYTES: usize = 32 * 1024;
const EVENT_BUFFER_CAPACITY: usize = 256;

type PendingResponse = mpsc::Sender<Result<Value, AdapterError>>;

#[derive(Debug)]
pub enum AdapterError {
    Spawn,
    Io,
    Protocol,
    Rpc { code: i64 },
    Timeout,
    Disconnected,
    InvalidInput,
    ThreadBusy,
}

impl AdapterError {
    pub fn public_code(&self) -> &'static str {
        match self {
            Self::Spawn => "CODEX_NOT_FOUND",
            Self::Io => "CODEX_IO_ERROR",
            Self::Protocol => "CODEX_PROTOCOL_ERROR",
            Self::Rpc { code: -32601 } => "CODEX_INCOMPATIBLE",
            Self::Rpc { .. } => "CODEX_RPC_ERROR",
            Self::Timeout => "CODEX_TIMEOUT",
            Self::Disconnected => "CODEX_DISCONNECTED",
            Self::InvalidInput => "COLLAB_INVALID_INPUT",
            Self::ThreadBusy => "COLLAB_THREAD_BUSY_EXTERNAL",
        }
    }
}

impl fmt::Display for AdapterError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(self.public_code())
    }
}

impl std::error::Error for AdapterError {}

#[derive(Debug, Clone)]
struct AppServerEvent {
    method: String,
    params: Value,
}

struct AppServerInner {
    stdin: Mutex<ChildStdin>,
    pending: Mutex<HashMap<u64, PendingResponse>>,
    event_tx: mpsc::SyncSender<AppServerEvent>,
    next_id: AtomicU64,
}

pub struct AppServerClient {
    inner: Arc<AppServerInner>,
    child: Mutex<Child>,
    events: Mutex<mpsc::Receiver<AppServerEvent>>,
    request_timeout: Duration,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct NormalizedThread {
    pub id: String,
    pub title: String,
    pub cwd_label: Option<String>,
    pub status: String,
    pub can_write: bool,
    pub updated_at: Option<i64>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ThreadPage {
    pub data: Vec<NormalizedThread>,
    pub next_cursor: Option<String>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StartedTask {
    pub thread_id: String,
    pub turn_id: String,
    pub status: String,
}

impl AppServerClient {
    pub fn start_default() -> Result<Self, AdapterError> {
        Self::spawn("codex", DEFAULT_REQUEST_TIMEOUT)
    }

    fn spawn(executable: &str, request_timeout: Duration) -> Result<Self, AdapterError> {
        let mut child = Command::new(executable)
            .arg("app-server")
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .map_err(|_| AdapterError::Spawn)?;
        let stdin = child.stdin.take().ok_or(AdapterError::Io)?;
        let stdout = child.stdout.take().ok_or(AdapterError::Io)?;
        let stderr = child.stderr.take().ok_or(AdapterError::Io)?;
        let (event_tx, event_rx) = mpsc::sync_channel(EVENT_BUFFER_CAPACITY);
        let inner = Arc::new(AppServerInner {
            stdin: Mutex::new(stdin),
            pending: Mutex::new(HashMap::new()),
            event_tx,
            next_id: AtomicU64::new(1),
        });

        start_stdout_reader(Arc::clone(&inner), stdout);
        // Drain stderr to avoid child-process backpressure. Its contents are
        // deliberately neither logged nor relayed because they can contain paths.
        thread::spawn(move || {
            for line in BufReader::new(stderr).lines() {
                if line.is_err() {
                    break;
                }
            }
        });

        let client = Self {
            inner,
            child: Mutex::new(child),
            events: Mutex::new(event_rx),
            request_timeout,
        };
        if let Err(error) = client.initialize() {
            client.stop_process();
            return Err(error);
        }
        Ok(client)
    }

    fn initialize(&self) -> Result<(), AdapterError> {
        self.request(
            "initialize",
            json!({
                "clientInfo": {
                    "name": "sub2api_codex_pc",
                    "title": "Sub2API Codex PC Companion",
                    "version": env!("CARGO_PKG_VERSION")
                },
                "capabilities": { "experimentalApi": false }
            }),
        )?;
        self.notify("initialized", json!({}))
    }

    pub fn list_threads(
        &self,
        limit: u32,
        cursor: Option<&str>,
        search_term: Option<&str>,
        archived: bool,
    ) -> Result<ThreadPage, AdapterError> {
        if limit == 0
            || limit > 100
            || cursor.is_some_and(|value| value.len() > 1024)
            || search_term.is_some_and(|value| value.len() > 200)
        {
            return Err(AdapterError::InvalidInput);
        }
        let raw = self.request(
            "thread/list",
            json!({
                "limit": limit,
                "cursor": cursor,
                "searchTerm": search_term,
                "sourceKinds": ["cli"],
                "archived": archived,
                "sortKey": "updated_at",
                "sortDirection": "desc"
            }),
        )?;
        normalize_thread_page(&raw)
    }

    pub fn start_task(&self, thread_id: &str, prompt: &str) -> Result<StartedTask, AdapterError> {
        let thread_id = thread_id.trim();
        if thread_id.is_empty()
            || thread_id.len() > 512
            || prompt.trim().is_empty()
            || prompt.len() > MAX_PROMPT_BYTES
        {
            return Err(AdapterError::InvalidInput);
        }
        let read = self.request(
            "thread/read",
            json!({"threadId": thread_id, "includeTurns": false}),
        )?;
        let status = read
            .pointer("/thread/status/type")
            .and_then(Value::as_str)
            .ok_or(AdapterError::Protocol)?;
        if status == "active" {
            return Err(AdapterError::ThreadBusy);
        }
        if status == "systemError" {
            return Err(AdapterError::Protocol);
        }
        self.request("thread/resume", json!({"threadId": thread_id}))?;
        let started = self.request(
            "turn/start",
            json!({
                "threadId": thread_id,
                "input": [{"type": "text", "text": prompt}],
                "approvalPolicy": "never"
            }),
        )?;
        let turn_id = started
            .pointer("/turn/id")
            .and_then(Value::as_str)
            .ok_or(AdapterError::Protocol)?;
        let turn_status = started
            .pointer("/turn/status")
            .and_then(Value::as_str)
            .unwrap_or("inProgress");
        Ok(StartedTask {
            thread_id: thread_id.to_owned(),
            turn_id: turn_id.to_owned(),
            status: turn_status.to_owned(),
        })
    }

    pub fn interrupt(&self, thread_id: &str, turn_id: &str) -> Result<(), AdapterError> {
        if thread_id.trim().is_empty()
            || thread_id.len() > 512
            || turn_id.trim().is_empty()
            || turn_id.len() > 512
        {
            return Err(AdapterError::InvalidInput);
        }
        self.request(
            "turn/interrupt",
            json!({"threadId": thread_id, "turnId": turn_id}),
        )?;
        Ok(())
    }

    #[allow(dead_code)]
    fn try_next_event(&self) -> Result<Option<(String, Value)>, AdapterError> {
        let receiver = self.events.lock().map_err(|_| AdapterError::Disconnected)?;
        match receiver.try_recv() {
            Ok(event) => Ok(Some((event.method, event.params))),
            Err(mpsc::TryRecvError::Empty) => Ok(None),
            Err(mpsc::TryRecvError::Disconnected) => Err(AdapterError::Disconnected),
        }
    }

    fn request(&self, method: &str, params: Value) -> Result<Value, AdapterError> {
        let id = self.inner.next_id.fetch_add(1, Ordering::Relaxed);
        let (response_tx, response_rx) = mpsc::channel();
        self.inner
            .pending
            .lock()
            .map_err(|_| AdapterError::Disconnected)?
            .insert(id, response_tx);
        if write_message(
            &self.inner,
            &json!({"id": id, "method": method, "params": params}),
        )
        .is_err()
        {
            remove_pending(&self.inner, id);
            return Err(AdapterError::Io);
        }
        match response_rx.recv_timeout(self.request_timeout) {
            Ok(result) => result,
            Err(mpsc::RecvTimeoutError::Timeout) => {
                remove_pending(&self.inner, id);
                Err(AdapterError::Timeout)
            }
            Err(mpsc::RecvTimeoutError::Disconnected) => Err(AdapterError::Disconnected),
        }
    }

    fn notify(&self, method: &str, params: Value) -> Result<(), AdapterError> {
        write_message(&self.inner, &json!({"method": method, "params": params}))
    }

    fn stop_process(&self) {
        if let Ok(mut child) = self.child.lock() {
            let _ = child.kill();
            let _ = child.wait();
        }
    }
}

impl Drop for AppServerClient {
    fn drop(&mut self) {
        self.stop_process();
    }
}

fn start_stdout_reader(inner: Arc<AppServerInner>, stdout: std::process::ChildStdout) {
    thread::spawn(move || {
        for line in BufReader::new(stdout).lines() {
            let Ok(line) = line else {
                break;
            };
            let Ok(message) = serde_json::from_str::<Value>(&line) else {
                continue;
            };
            route_message(&inner, message);
        }
        if let Ok(mut pending) = inner.pending.lock() {
            for (_, sender) in pending.drain() {
                let _ = sender.send(Err(AdapterError::Disconnected));
            }
        }
    });
}

fn route_message(inner: &Arc<AppServerInner>, message: Value) {
    let id = message.get("id").cloned();
    let method = message.get("method").and_then(Value::as_str);
    if let (Some(id), Some(method)) = (id, method) {
        let response = non_interactive_server_response(id, method);
        let _ = write_message(inner, &response);
        let _ = inner.event_tx.try_send(AppServerEvent {
            method: "adapter/serverRequestRejected".to_owned(),
            params: json!({"method": method}),
        });
        return;
    }
    if let Some(id) = message.get("id").and_then(Value::as_u64) {
        let sender = inner
            .pending
            .lock()
            .ok()
            .and_then(|mut pending| pending.remove(&id));
        if let Some(sender) = sender {
            let result = if let Some(error) = message.get("error") {
                Err(AdapterError::Rpc {
                    code: error.get("code").and_then(Value::as_i64).unwrap_or(-32000),
                })
            } else {
                message.get("result").cloned().ok_or(AdapterError::Protocol)
            };
            let _ = sender.send(result);
        }
        return;
    }
    if let Some(method) = method {
        let _ = inner.event_tx.try_send(AppServerEvent {
            method: method.to_owned(),
            params: message.get("params").cloned().unwrap_or_else(|| json!({})),
        });
    }
}

fn non_interactive_server_response(id: Value, method: &str) -> Value {
    match method {
        "item/commandExecution/requestApproval" | "item/fileChange/requestApproval" => {
            json!({"id": id, "result": {"decision": "decline"}})
        }
        "mcpServer/elicitation/request" => {
            json!({"id": id, "result": {"action": "decline", "content": null}})
        }
        _ => json!({
            "id": id,
            "error": {
                "code": -32601,
                "message": "Unsupported by non-interactive companion"
            }
        }),
    }
}

fn normalize_thread_page(raw: &Value) -> Result<ThreadPage, AdapterError> {
    let data = raw
        .get("data")
        .and_then(Value::as_array)
        .ok_or(AdapterError::Protocol)?;
    let mut threads = Vec::with_capacity(data.len());
    for thread in data {
        let id = thread
            .get("id")
            .and_then(Value::as_str)
            .ok_or(AdapterError::Protocol)?;
        let title = thread
            .get("name")
            .and_then(Value::as_str)
            .or_else(|| thread.get("preview").and_then(Value::as_str))
            .unwrap_or("Untitled thread");
        let status = thread
            .pointer("/status/type")
            .and_then(Value::as_str)
            .unwrap_or("notLoaded");
        threads.push(NormalizedThread {
            id: id.to_owned(),
            title: truncate_chars(title, 200),
            cwd_label: thread
                .get("cwd")
                .and_then(Value::as_str)
                .map(sanitize_path_label),
            status: status.to_owned(),
            can_write: status == "idle" || status == "notLoaded",
            updated_at: thread.get("updatedAt").and_then(Value::as_i64),
        });
    }
    Ok(ThreadPage {
        data: threads,
        next_cursor: raw
            .get("nextCursor")
            .and_then(Value::as_str)
            .map(str::to_owned),
    })
}

fn sanitize_path_label(path: &str) -> String {
    let normalized = path.replace('\\', "/");
    let leaf = normalized
        .trim_end_matches('/')
        .rsplit('/')
        .find(|part| !part.is_empty())
        .unwrap_or("workspace");
    format!("…/{}", truncate_chars(leaf, 80))
}

fn truncate_chars(value: &str, max_chars: usize) -> String {
    value.chars().take(max_chars).collect()
}

fn write_message(inner: &AppServerInner, message: &Value) -> Result<(), AdapterError> {
    let encoded = serde_json::to_vec(message).map_err(|_| AdapterError::Protocol)?;
    let mut stdin = inner.stdin.lock().map_err(|_| AdapterError::Disconnected)?;
    stdin.write_all(&encoded).map_err(|_| AdapterError::Io)?;
    stdin.write_all(b"\n").map_err(|_| AdapterError::Io)?;
    stdin.flush().map_err(|_| AdapterError::Io)
}

fn remove_pending(inner: &AppServerInner, id: u64) {
    if let Ok(mut pending) = inner.pending.lock() {
        pending.remove(&id);
    }
}

#[cfg(test)]
mod tests {
    use super::non_interactive_server_response;
    use super::normalize_thread_page;
    use serde_json::json;

    #[test]
    fn rejects_approval_requests_without_a_pc_confirmation_flow() {
        assert_eq!(
            non_interactive_server_response(json!(7), "item/commandExecution/requestApproval"),
            json!({"id": 7, "result": {"decision": "decline"}})
        );
        assert_eq!(
            non_interactive_server_response(json!(8), "item/permissions/requestApproval"),
            json!({"id": 8, "error": {"code": -32601, "message": "Unsupported by non-interactive companion"}})
        );
    }

    #[test]
    fn normalizes_threads_without_exposing_absolute_paths() {
        let page = normalize_thread_page(&json!({
            "data": [{
                "id": "thread_1",
                "preview": "Fix login",
                "cwd": "/home/alice/private/project",
                "updatedAt": 123,
                "status": {"type": "notLoaded"}
            }],
            "nextCursor": "next"
        }))
        .expect("thread page should normalize");
        assert_eq!(page.data[0].cwd_label.as_deref(), Some("…/project"));
        assert!(page.data[0].can_write);
        assert_eq!(page.next_cursor.as_deref(), Some("next"));
    }
}
