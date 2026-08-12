//! Minimal Codex app-server stdio adapter.
//!
//! Raw app-server messages stay inside this module. Callers receive normalized
//! thread metadata or bounded task identifiers, never stderr or prompt echoes.

use serde::Serialize;
use serde_json::json;
use serde_json::Value;
use std::collections::HashMap;
use std::collections::HashSet;
use std::collections::VecDeque;
use std::env;
use std::fmt;
use std::fs;
use std::io::BufRead;
use std::io::BufReader;
use std::io::Write;
use std::path::Path;
use std::path::PathBuf;
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
use std::time::Instant;

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

#[derive(Debug, Clone, PartialEq)]
pub struct NormalizedCompletedItem {
    pub item_id: String,
    pub item_type: String,
    pub role: Option<String>,
    pub title: Option<String>,
    pub summary: Option<String>,
    pub text: Option<String>,
    pub status: String,
    pub completed_at_ms: i64,
}

#[derive(Debug, Clone, PartialEq)]
pub struct TurnCompletion {
    pub status: String,
    pub items: Vec<NormalizedCompletedItem>,
}

#[derive(Debug, Clone, PartialEq)]
pub struct NormalizedHistoricalItem {
    pub turn_id: String,
    pub item: NormalizedCompletedItem,
}

#[derive(Debug, Clone, PartialEq)]
pub struct NormalizedThreadSnapshot {
    pub thread_id: String,
    pub title: String,
    pub status: String,
    pub write_state: String,
    pub items: Vec<NormalizedHistoricalItem>,
}

impl AppServerClient {
    pub fn start_default() -> Result<Self, AdapterError> {
        for executable in codex_executable_candidates() {
            if !executable.is_file() {
                continue;
            }
            match Self::spawn(&executable, DEFAULT_REQUEST_TIMEOUT) {
                Ok(client) => return Ok(client),
                Err(AdapterError::Spawn) => continue,
                Err(error) => return Err(error),
            }
        }
        Err(AdapterError::Spawn)
    }

    fn spawn(executable: &Path, request_timeout: Duration) -> Result<Self, AdapterError> {
        let mut command = Command::new(executable);
        prepend_executable_directory_to_path(&mut command, executable);
        let mut child = command
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
        self.start_task_with_client_message_id(thread_id, prompt, None)
    }

    pub fn start_task_with_client_message_id(
        &self,
        thread_id: &str,
        prompt: &str,
        client_message_id: Option<&str>,
    ) -> Result<StartedTask, AdapterError> {
        let thread_id = thread_id.trim();
        if thread_id.is_empty()
            || thread_id.len() > 512
            || prompt.trim().is_empty()
            || prompt.len() > MAX_PROMPT_BYTES
            || client_message_id.is_some_and(|value| value.is_empty() || value.len() > 512)
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
                "clientUserMessageId": client_message_id,
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

    pub fn wait_for_turn_completion(
        &self,
        turn_id: &str,
        timeout: Duration,
    ) -> Result<TurnCompletion, AdapterError> {
        if turn_id.trim().is_empty() || turn_id.len() > 512 || timeout.is_zero() {
            return Err(AdapterError::InvalidInput);
        }
        let deadline = Instant::now() + timeout;
        let receiver = self.events.lock().map_err(|_| AdapterError::Disconnected)?;
        let mut items = Vec::new();
        loop {
            let remaining = deadline
                .checked_duration_since(Instant::now())
                .ok_or(AdapterError::Timeout)?;
            let event = receiver
                .recv_timeout(remaining)
                .map_err(|error| match error {
                    mpsc::RecvTimeoutError::Timeout => AdapterError::Timeout,
                    mpsc::RecvTimeoutError::Disconnected => AdapterError::Disconnected,
                })?;
            if event.params.get("turnId").and_then(Value::as_str) != Some(turn_id) {
                continue;
            }
            if event.method == "item/completed" {
                if items.len() < 200 {
                    if let Some(item) = normalize_completed_item(&event.params) {
                        items.push(item);
                    }
                }
                continue;
            }
            if event.method != "turn/completed" {
                continue;
            }
            let status = event
                .params
                .pointer("/turn/status")
                .and_then(Value::as_str)
                .ok_or(AdapterError::Protocol)?;
            return Ok(TurnCompletion {
                status: status.to_owned(),
                items,
            });
        }
    }

    pub fn read_thread_snapshot(
        &self,
        thread_id: &str,
        after_item_id: Option<&str>,
        limit: usize,
    ) -> Result<NormalizedThreadSnapshot, AdapterError> {
        let thread_id = thread_id.trim();
        if thread_id.is_empty()
            || thread_id.len() > 512
            || after_item_id.is_some_and(|value| value.is_empty() || value.len() > 512)
            || limit == 0
            || limit > 200
        {
            return Err(AdapterError::InvalidInput);
        }
        let raw = self.request(
            "thread/read",
            json!({"threadId": thread_id, "includeTurns": true}),
        )?;
        normalize_thread_snapshot(&raw, thread_id, after_item_id, limit)
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

fn codex_executable_candidates() -> Vec<PathBuf> {
    let mut candidates = Vec::new();
    if let Some(explicit) = env::var_os("CODEX_PATH") {
        candidates.push(PathBuf::from(explicit));
    }
    if let Some(path) = env::var_os("PATH") {
        candidates.extend(env::split_paths(&path).map(|directory| directory.join("codex")));
    }
    candidates.extend([
        PathBuf::from("/usr/local/bin/codex"),
        PathBuf::from("/usr/bin/codex"),
        PathBuf::from("/snap/bin/codex"),
    ]);

    if let Some(home) = env::var_os("HOME").map(PathBuf::from) {
        candidates.extend([
            home.join(".local/bin/codex"),
            home.join(".npm-global/bin/codex"),
            home.join(".local/share/pnpm/codex"),
            home.join(".bun/bin/codex"),
        ]);
        append_versioned_codex(
            &mut candidates,
            &home.join(".nvm/versions/node"),
            "bin/codex",
        );
        for extension_root in [
            ".codebuddycn/extensions",
            ".vscode/extensions",
            ".vscode-server/extensions",
            ".cursor/extensions",
            ".cursor-server/extensions",
        ] {
            append_openai_extension_codex(&mut candidates, &home.join(extension_root));
        }
    }

    let mut seen = HashSet::new();
    candidates.retain(|candidate| seen.insert(candidate.clone()));
    candidates
}

fn append_versioned_codex(candidates: &mut Vec<PathBuf>, root: &Path, suffix: &str) {
    let Ok(entries) = fs::read_dir(root) else {
        return;
    };
    let mut versions = entries
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .collect::<Vec<_>>();
    versions.sort_by(|left, right| right.file_name().cmp(&left.file_name()));
    candidates.extend(versions.into_iter().map(|version| version.join(suffix)));
}

fn append_openai_extension_codex(candidates: &mut Vec<PathBuf>, root: &Path) {
    let Ok(entries) = fs::read_dir(root) else {
        return;
    };
    let mut extensions = entries
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .filter(|path| {
            path.file_name()
                .and_then(|name| name.to_str())
                .is_some_and(|name| name.starts_with("openai.chatgpt-"))
        })
        .collect::<Vec<_>>();
    extensions.sort_by(|left, right| right.file_name().cmp(&left.file_name()));
    candidates.extend(
        extensions
            .into_iter()
            .map(|extension| extension.join("bin/linux-x86_64/codex")),
    );
}

fn prepend_executable_directory_to_path(command: &mut Command, executable: &Path) {
    let Some(parent) = executable.parent() else {
        return;
    };
    let current = env::var_os("PATH").unwrap_or_default();
    let paths = std::iter::once(parent.to_path_buf()).chain(env::split_paths(&current));
    if let Ok(path) = env::join_paths(paths) {
        command.env("PATH", path);
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

fn normalize_completed_item(params: &Value) -> Option<NormalizedCompletedItem> {
    let item = params.get("item")?;
    let completed_at_ms = params
        .get("completedAtMs")
        .and_then(Value::as_i64)
        .unwrap_or_default();
    normalize_item(item, completed_at_ms)
}

fn normalize_item(item: &Value, completed_at_ms: i64) -> Option<NormalizedCompletedItem> {
    let item_id = item.get("id")?.as_str()?;
    if item_id.is_empty() || item_id.len() > 512 {
        return None;
    }
    let raw_type = item.get("type").and_then(Value::as_str).unwrap_or_default();
    let (item_type, role, title, summary, text, status) = match raw_type {
        "userMessage" => {
            let text = item
                .get("content")
                .and_then(Value::as_array)
                .into_iter()
                .flatten()
                .filter(|part| part.get("type").and_then(Value::as_str) == Some("text"))
                .filter_map(|part| part.get("text").and_then(Value::as_str))
                .collect::<Vec<_>>()
                .join("\n");
            (
                "user_message",
                Some("user"),
                None,
                None,
                bounded_text(&text),
                "completed",
            )
        }
        "agentMessage" => (
            "agent_message",
            Some("assistant"),
            None,
            None,
            item.get("text")
                .and_then(Value::as_str)
                .and_then(bounded_text),
            "completed",
        ),
        "reasoning" => {
            let summary = item
                .get("summary")
                .and_then(Value::as_array)
                .into_iter()
                .flatten()
                .filter_map(Value::as_str)
                .collect::<Vec<_>>()
                .join("\n");
            (
                "reasoning_summary",
                None,
                Some("推理摘要"),
                bounded_text(&summary),
                None,
                "completed",
            )
        }
        "plan" => (
            "plan",
            None,
            Some("计划"),
            None,
            item.get("text")
                .and_then(Value::as_str)
                .and_then(bounded_text),
            "completed",
        ),
        "commandExecution" => {
            let item_status = item
                .get("status")
                .and_then(Value::as_str)
                .unwrap_or("completed");
            let mut safe_summary = item_status.to_owned();
            if let Some(exit_code) = item.get("exitCode").and_then(Value::as_i64) {
                safe_summary.push_str(&format!(" · exit {exit_code}"));
            }
            if let Some(duration_ms) = item.get("durationMs").and_then(Value::as_i64) {
                safe_summary.push_str(&format!(" · {duration_ms} ms"));
            }
            (
                "command_execution",
                None,
                Some("命令执行"),
                bounded_text(&safe_summary),
                None,
                item_status,
            )
        }
        "fileChange" => {
            let count = item
                .get("changes")
                .and_then(Value::as_array)
                .map_or(0, Vec::len);
            (
                "file_change",
                None,
                Some("文件变更"),
                Some(format!("{count} 项变更")),
                None,
                item.get("status")
                    .and_then(Value::as_str)
                    .unwrap_or("completed"),
            )
        }
        "mcpToolCall" | "dynamicToolCall" => (
            "tool_call",
            None,
            Some("工具调用"),
            item.get("status")
                .and_then(Value::as_str)
                .and_then(bounded_text),
            None,
            item.get("status")
                .and_then(Value::as_str)
                .unwrap_or("completed"),
        ),
        _ => (
            "unsupported",
            None,
            Some("未支持的事件"),
            None,
            None,
            "completed",
        ),
    };
    Some(NormalizedCompletedItem {
        item_id: item_id.to_owned(),
        item_type: item_type.to_owned(),
        role: role.map(str::to_owned),
        title: title.map(str::to_owned),
        summary,
        text,
        status: truncate_chars(status, 64),
        completed_at_ms,
    })
}

fn normalize_thread_snapshot(
    raw: &Value,
    expected_thread_id: &str,
    after_item_id: Option<&str>,
    limit: usize,
) -> Result<NormalizedThreadSnapshot, AdapterError> {
    let thread = raw.get("thread").ok_or(AdapterError::Protocol)?;
    let thread_id = thread
        .get("id")
        .and_then(Value::as_str)
        .ok_or(AdapterError::Protocol)?;
    if thread_id != expected_thread_id {
        return Err(AdapterError::Protocol);
    }
    let title = thread
        .get("name")
        .and_then(Value::as_str)
        .or_else(|| thread.get("preview").and_then(Value::as_str))
        .unwrap_or("Untitled thread");
    let status = thread
        .pointer("/status/type")
        .and_then(Value::as_str)
        .unwrap_or("notLoaded");
    let write_state = if status == "active" {
        "busy"
    } else if status == "notLoaded" {
        "writable_resumable"
    } else if status == "idle" {
        "writable_loaded"
    } else {
        "read_only"
    };
    let turns = thread
        .get("turns")
        .and_then(Value::as_array)
        .ok_or(AdapterError::Protocol)?;
    let mut found_after = after_item_id.is_none();
    let mut after_items = Vec::with_capacity(limit);
    let mut fallback = VecDeque::with_capacity(limit);
    for turn in turns {
        let Some(turn_id) = turn.get("id").and_then(Value::as_str) else {
            continue;
        };
        let completed_at_ms = turn
            .get("completedAt")
            .and_then(Value::as_i64)
            .or_else(|| turn.get("startedAt").and_then(Value::as_i64))
            .unwrap_or_default()
            .saturating_mul(1000);
        let Some(items) = turn.get("items").and_then(Value::as_array) else {
            continue;
        };
        for raw_item in items {
            let Some(item) = normalize_item(raw_item, completed_at_ms) else {
                continue;
            };
            if fallback.len() == limit {
                fallback.pop_front();
            }
            fallback.push_back(NormalizedHistoricalItem {
                turn_id: turn_id.to_owned(),
                item: item.clone(),
            });
            if !found_after {
                if after_item_id == Some(item.item_id.as_str()) {
                    found_after = true;
                }
                continue;
            }
            if after_items.len() < limit {
                after_items.push(NormalizedHistoricalItem {
                    turn_id: turn_id.to_owned(),
                    item,
                });
            }
        }
    }
    let items = if after_item_id.is_some() && !found_after {
        fallback.into_iter().collect()
    } else if after_item_id.is_none() {
        fallback.into_iter().collect()
    } else {
        after_items
    };
    Ok(NormalizedThreadSnapshot {
        thread_id: thread_id.to_owned(),
        title: truncate_chars(title, 200),
        status: status.to_owned(),
        write_state: write_state.to_owned(),
        items,
    })
}

fn bounded_text(value: &str) -> Option<String> {
    let value = value.trim();
    if value.is_empty() {
        return None;
    }
    Some(truncate_chars(value, 64_000))
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
    use super::append_openai_extension_codex;
    use super::append_versioned_codex;
    use super::non_interactive_server_response;
    use super::normalize_completed_item;
    use super::normalize_thread_page;
    use super::normalize_thread_snapshot;
    use serde_json::json;
    use std::fs;
    use std::path::PathBuf;
    use std::time::SystemTime;

    fn temporary_directory(label: &str) -> PathBuf {
        let nonce = SystemTime::now()
            .duration_since(SystemTime::UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        std::env::temp_dir().join(format!("sub2api-{label}-{}-{nonce}", std::process::id()))
    }

    #[test]
    fn discovers_codex_installed_by_nvm() {
        let root = temporary_directory("nvm");
        let executable = root.join("v20.20.0/bin/codex");
        fs::create_dir_all(executable.parent().unwrap()).unwrap();
        fs::write(&executable, b"#!/bin/sh\n").unwrap();

        let mut candidates = Vec::new();
        append_versioned_codex(&mut candidates, &root, "bin/codex");

        assert_eq!(candidates, vec![executable]);
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn discovers_codex_bundled_with_openai_editor_extension() {
        let root = temporary_directory("extension");
        let executable = root.join("openai.chatgpt-1.2.3/bin/linux-x86_64/codex");
        fs::create_dir_all(executable.parent().unwrap()).unwrap();
        fs::write(&executable, b"binary").unwrap();

        let mut candidates = Vec::new();
        append_openai_extension_codex(&mut candidates, &root);

        assert_eq!(candidates, vec![executable]);
        fs::remove_dir_all(root).unwrap();
    }

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

    #[test]
    fn completed_command_item_drops_command_paths_and_output() {
        let item = normalize_completed_item(&json!({
            "threadId": "thread_1",
            "turnId": "turn_1",
            "completedAtMs": 123,
            "item": {
                "type": "commandExecution",
                "id": "item_1",
                "command": "cat /home/alice/private.txt",
                "cwd": "/home/alice",
                "aggregatedOutput": "refresh_token=secret",
                "status": "completed",
                "exitCode": 0,
                "durationMs": 80
            }
        }))
        .unwrap();
        assert_eq!(item.item_type, "command_execution");
        assert_eq!(item.title.as_deref(), Some("命令执行"));
        assert_eq!(item.summary.as_deref(), Some("completed · exit 0 · 80 ms"));
        let debug = format!("{item:?}");
        assert!(!debug.contains("private.txt"));
        assert!(!debug.contains("refresh_token"));
    }

    #[test]
    fn completed_agent_item_keeps_only_bounded_text() {
        let item = normalize_completed_item(&json!({
            "turnId": "turn_1",
            "completedAtMs": 123,
            "item": {"type": "agentMessage", "id": "item_2", "text": "Done"}
        }))
        .unwrap();
        assert_eq!(item.item_type, "agent_message");
        assert_eq!(item.role.as_deref(), Some("assistant"));
        assert_eq!(item.text.as_deref(), Some("Done"));
    }

    #[test]
    fn thread_snapshot_keeps_recent_normalized_items_only() {
        let snapshot = normalize_thread_snapshot(
            &json!({
                "thread": {
                    "id": "thread_1",
                    "name": "Fix login",
                    "status": {"type": "notLoaded"},
                    "turns": [{
                        "id": "turn_1",
                        "completedAt": 123,
                        "items": [
                            {"type": "userMessage", "id": "item_1", "content": [{"type": "text", "text": "one"}]},
                            {"type": "agentMessage", "id": "item_2", "text": "two"},
                            {"type": "agentMessage", "id": "item_3", "text": "three"}
                        ]
                    }]
                }
            }),
            "thread_1",
            None,
            2,
        )
        .unwrap();
        assert_eq!(snapshot.write_state, "writable_resumable");
        assert_eq!(snapshot.items.len(), 2);
        assert_eq!(snapshot.items[0].item.item_id, "item_2");
        assert_eq!(snapshot.items[1].item.item_id, "item_3");
    }

    #[test]
    fn thread_snapshot_after_item_returns_incremental_items() {
        let snapshot = normalize_thread_snapshot(
            &json!({
                "thread": {
                    "id": "thread_1",
                    "status": {"type": "idle"},
                    "turns": [{
                        "id": "turn_1",
                        "items": [
                            {"type": "agentMessage", "id": "item_1", "text": "one"},
                            {"type": "agentMessage", "id": "item_2", "text": "two"}
                        ]
                    }]
                }
            }),
            "thread_1",
            Some("item_1"),
            10,
        )
        .unwrap();
        assert_eq!(snapshot.items.len(), 1);
        assert_eq!(snapshot.items[0].item.item_id, "item_2");
    }
}
