package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// codexFingerprintIDsContextKey 是暂存在 gin context 的收敛 ID 集合键。
// 由 Forward（非透传）或 forwardOpenAIPassthrough（透传）解析后写入，请求
// 构造器读取用于出站头改写——请求体与出站头必须共享同一份 IDs，保证
// turn_id 等随机字段一致。
const codexFingerprintIDsContextKey = "codex_fingerprint_ids"

// stageCodexFingerprintIDs 将本 attempt 解析出的收敛 ID 暂存到 gin context。
// 必须无条件覆写（含 nil）：failover 从收敛账号切到 off 账号时，上一账号的
// IDs 不得残留并被误应用到新账号的出站头（typed-nil 由应用侧 nil 守卫吸收）。
func stageCodexFingerprintIDs(c *gin.Context, ids *codexFingerprintIDs) {
	if c != nil {
		c.Set(codexFingerprintIDsContextKey, ids)
	}
}

func stagedCodexFingerprintIDs(c *gin.Context, account *Account) *codexFingerprintIDs {
	if c == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return nil
	}
	value, ok := c.Get(codexFingerprintIDsContextKey)
	if !ok {
		return nil
	}
	ids, ok := value.(*codexFingerprintIDs)
	if !ok || ids == nil || ids.accountID != account.ID {
		return nil
	}
	return ids
}

// applyStagedCodexFingerprintHeaders 读取 context 暂存的收敛 ID 并改写出站头。
// 非透传与透传两个请求构造器共用本函数，防止应用语义漂移。仅解析该
// snapshot 的 OAuth 账号可读取，避免 stale context 跨账号 failover 泄漏。
func applyStagedCodexFingerprintHeaders(c *gin.Context, account *Account, h http.Header) {
	applyCodexFingerprintHeaders(h, stagedCodexFingerprintIDs(c, account))
}

func applyStagedCodexFingerprintClientMetadata(c *gin.Context, account *Account, reqBody map[string]any) bool {
	return applyCodexFingerprintClientMetadata(reqBody, stagedCodexFingerprintIDs(c, account))
}

// codexFingerprintMode 控制 OAuth 账号出站请求的设备指纹收敛强度。
// 多人共享同一 OAuth 账号时，每个用户的 Codex 客户端会携带各自不同的
// installation_id / session_id / thread_id，上游据此判定设备数和会话数。
// 收敛模式将这些标识改写为账号级恒定值，减少上游可见的设备/会话指纹。
type codexFingerprintMode string

const (
	// codexFingerprintOff 不做任何收敛，原样透传客户端标识。仅管理员显式
	// 配置 off 时使用；缺省账号走 session（见 GetCodexFingerprintMode）。
	codexFingerprintOff codexFingerprintMode = "off"
	// codexFingerprintDevice 仅收敛 installation_id 为账号级恒定值。
	// 上游看到 1 台设备 + 多会话（每用户各自的 session）。
	codexFingerprintDevice codexFingerprintMode = "device"
	// codexFingerprintSession 收敛 installation_id + session_id，
	// thread_id 按客户端原始 session-id 确定性派生（每个真实 Codex 会话一个独立线程）。
	// 上游看到 1 台设备 + 1 会话 + N 线程，最接近正常用户 spawn 子代理的模式。
	codexFingerprintSession codexFingerprintMode = "session"
	// codexFingerprintFull 收敛所有标识：installation_id + session_id + thread_id。
	// 上游看到 1 台设备 + 1 会话 + 1 线程，最激进。
	codexFingerprintFull codexFingerprintMode = "full"
)

const (
	codexFingerprintModeExtraKey = "codex_fingerprint_mode"
	codexFingerprintSeedExtraKey = "codex_fingerprint_seed"
)

// FORK-UPSTREAM-PRECEDENCE(v0.1.185-audit): 以下默认收敛、部署域假名化和
// metadata 闭集收口用于补齐上游仍未覆盖的客户端原值泄漏面。v0.1.181 新增的
// account identity scoping 已保留并先于本层执行，但它仍默认 off、缺少部署域
// 密钥且不会闭集清理 metadata/传输旁路，因此不等价。若 Wei-Shaw/sub2api
// 后续提供等价修复，应删除本 fork 实现及配套迁移/测试，直接采用上游语义。

// codexFingerprintDeploymentKeyHash 是部署域密钥的进程级摘要。账号 seed 会随
// 数据库克隆，但独立部署通常有不同的专用域密钥或 JWT secret；将部署域纳入
// 派生后，克隆库不会继续产生相同 installation/session/thread 指纹。
var codexFingerprintDeploymentKeyHash atomic.Pointer[[sha256.Size]byte]

// SetCodexFingerprintDeploymentKey 发布稳定的部署域密钥。生产环境优先使用
// gateway.codex_fingerprint_domain_key；未配置时由调用方传入 JWT secret。
func SetCodexFingerprintDeploymentKey(key string) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		codexFingerprintDeploymentKeyHash.Store(nil)
		return
	}
	digest := sha256.Sum256([]byte("sub2api:codex-fingerprint-domain:v1\x00" + trimmed))
	codexFingerprintDeploymentKeyHash.Store(&digest)
}

func deriveCodexFingerprintUUID(label string, parts ...string) string {
	key := []byte("sub2api:codex-fingerprint-unconfigured-domain:v1")
	if configured := codexFingerprintDeploymentKeyHash.Load(); configured != nil {
		key = configured[:]
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(label))
	for _, part := range parts {
		_, _ = mac.Write([]byte{0})
		_, _ = mac.Write([]byte(part))
	}
	return stableUUIDv4FromDigest(mac.Sum(nil))
}

func canonicalCodexFingerprintSeed(value any) (string, bool) {
	raw, ok := value.(string)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(raw)
	parsed, err := uuid.Parse(trimmed)
	if err != nil || parsed == uuid.Nil || trimmed != parsed.String() {
		return "", false
	}
	return trimmed, true
}

func newCodexFingerprintSeed() string {
	return uuid.NewString()
}

func stripCodexFingerprintSeed(extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}
	stripped := maps.Clone(extra)
	delete(stripped, codexFingerprintSeedExtraKey)
	return stripped
}

func codexFingerprintModeFromExtra(extra map[string]any) codexFingerprintMode {
	if extra == nil {
		return codexFingerprintSession
	}
	raw, _ := extra[codexFingerprintModeExtraKey].(string)
	switch codexFingerprintMode(strings.TrimSpace(raw)) {
	case codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull:
		return codexFingerprintMode(strings.TrimSpace(raw))
	default:
		return codexFingerprintSession
	}
}

func codexFingerprintModeRequiresSeed(mode codexFingerprintMode) bool {
	switch mode {
	case codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull:
		return true
	default:
		return false
	}
}

func codexFingerprintSeed(extra map[string]any) (string, bool) {
	if extra == nil {
		return "", false
	}
	return canonicalCodexFingerprintSeed(extra[codexFingerprintSeedExtraKey])
}

func prepareCodexFingerprintExtraForCreate(platform, accountType string, extra map[string]any) map[string]any {
	prepared := stripCodexFingerprintSeed(extra)
	if platform != PlatformOpenAI || (accountType != AccountTypeOAuth && accountType != AccountTypeSetupToken) || !codexFingerprintModeRequiresSeed(codexFingerprintModeFromExtra(prepared)) {
		return prepared
	}
	if prepared == nil {
		prepared = make(map[string]any, 1)
	}
	prepared[codexFingerprintSeedExtraKey] = newCodexFingerprintSeed()
	return prepared
}

func prepareCodexFingerprintExtraForUpdate(account *Account, extra map[string]any) map[string]any {
	prepared := stripCodexFingerprintSeed(extra)
	if account == nil || !account.IsOpenAIOAuthLike() {
		return prepared
	}
	if seed, ok := codexFingerprintSeed(account.Extra); ok {
		if prepared == nil {
			prepared = make(map[string]any, 1)
		}
		prepared[codexFingerprintSeedExtraKey] = seed
		return prepared
	}
	if codexFingerprintModeRequiresSeed(codexFingerprintModeFromExtra(prepared)) {
		if prepared == nil {
			prepared = make(map[string]any, 1)
		}
		prepared[codexFingerprintSeedExtraKey] = newCodexFingerprintSeed()
	}
	return prepared
}

func sanitizedCodexFingerprintExtraUpdates(updates map[string]any) map[string]any {
	if updates == nil {
		return nil
	}
	sanitized := maps.Clone(updates)
	delete(sanitized, codexFingerprintSeedExtraKey)
	return sanitized
}

// ShouldEnsureCodexFingerprintSeedForExtraUpdates reports whether a JSONB key-level
// extra update is enabling Codex fingerprint convergence and therefore must atomically
// preserve or create the system-managed per-account seed in the repository update.
func ShouldEnsureCodexFingerprintSeedForExtraUpdates(updates map[string]any) bool {
	if updates == nil {
		return false
	}
	if _, explicitlyUpdated := updates[codexFingerprintModeExtraKey]; !explicitlyUpdated {
		return false
	}
	return codexFingerprintModeRequiresSeed(codexFingerprintModeFromExtra(updates))
}

// GetCodexFingerprintMode 从账号 extra JSON 读取指纹收敛模式。
//
// fork 默认采用 session：未设置、空值或非法值均收敛；只有管理员显式配置
// off 才允许客户端标识原样出站。该默认值有意覆盖上游 #5610 的 opt-in 语义，
// 原因是本部署要求客户端原值默认不离开网关。若上游后续提供正式的严格模式，
// 按 FORK-UPSTREAM-PRECEDENCE 注释整体替换本策略。
func (a *Account) GetCodexFingerprintMode() codexFingerprintMode {
	if a == nil || !a.IsOpenAIOAuthLike() {
		return codexFingerprintOff
	}
	return codexFingerprintModeFromExtra(a.Extra)
}

// deriveStableUUIDv4 从种子确定性派生一个 UUIDv4 格式的字符串。
// 同一种子永远返回同一值。
func deriveStableUUIDv4(seed string) string {
	h := sha256.Sum256([]byte(seed))
	return stableUUIDv4FromDigest(h[:])
}

func stableUUIDv4FromDigest(digest []byte) string {
	b := append([]byte(nil), digest[:16]...)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16])
}

// resolveConvergedInstallationID 返回账号级、部署域内恒定的 installation_id。
// 管理员配置的真实 device_id 只作为派生输入，绝不直接出站。
func resolveConvergedInstallationID(account *Account, seed string) string {
	if account == nil || seed == "" {
		return ""
	}
	if deviceID := account.GetOpenAIDeviceID(); deviceID != "" {
		return deriveCodexFingerprintUUID("installation:configured-device:v3", seed, deviceID)
	}
	return deriveCodexFingerprintUUID("installation:seed:v3", seed)
}

// resolvePseudonymousOpenAIDeviceID 用于正式收敛器之前的兼容注入点。即使账号
// 显式 off 或历史数据暂时缺少 seed，openai_device_id 原值也不得离开网关。
func resolvePseudonymousOpenAIDeviceID(account *Account) string {
	if account == nil {
		return ""
	}
	deviceID := account.GetOpenAIDeviceID()
	if deviceID == "" {
		return ""
	}
	seed, ok := codexFingerprintSeed(account.Extra)
	if !ok {
		seed = fmt.Sprintf("account:%d", account.ID)
	}
	return deriveCodexFingerprintUUID("installation:configured-device:v3", seed, deviceID)
}

// resolveConvergedSessionID 返回账号级恒定的 session_id。
func resolveConvergedSessionID(seed string) string {
	if seed == "" {
		return ""
	}
	return deriveCodexFingerprintUUID("session:v3", seed)
}

// resolveConvergedThreadID 按客户端原始 session-id 确定性派生 thread_id。
// 每个真实 Codex 会话（不同客户端启动实例）获得一个独立线程，
// 模拟正常用户 spawn 子代理或开多窗口的模式。
func resolveConvergedThreadID(seed, clientSessionID string) string {
	if seed == "" || clientSessionID == "" {
		return ""
	}
	return deriveCodexFingerprintUUID("thread:v3", seed, clientSessionID)
}

// codexFingerprintIDs 收敛后的完整 ID 集合。
// 由 resolveCodexFingerprintIDs 一次性生成，同一个实例在头改写和体改写之间共享，
// 确保所有载体中的 turn_id 等随机字段一致。体改写时还会补记原始
// client_metadata.session_id，用于识别 root prompt_cache_key 的默认值。
type codexFingerprintIDs struct {
	accountID                     int64
	mode                          codexFingerprintMode
	installationID                string
	sessionID                     string
	threadID                      string
	turnID                        string
	windowID                      string
	turnStartedAtUnixMs           int64
	originalBodySessionID         string
	originalBodySessionIDCaptured bool
}

// resolveCodexFingerprintIDs 按收敛模式计算出站 ID 集合。
// clientSessionID 是客户端原始的 session-id 头值（连字符形式），用于 session 模式下
// 的 thread_id 派生——每个真实 Codex 会话得到一个独立线程。
// 返回 nil 表示 off 模式，不需要改写。
// 注意：包含随机生成的 turn_id，调用方必须只调用一次并共享结果给头改写和体改写。
func resolveCodexFingerprintIDs(account *Account, clientSessionID string, mode codexFingerprintMode) *codexFingerprintIDs {
	if account == nil || mode == codexFingerprintOff {
		return nil
	}
	seed, ok := codexFingerprintSeed(account.Extra)
	if !ok {
		return nil
	}

	ids := &codexFingerprintIDs{
		accountID:           account.ID,
		mode:                mode,
		turnStartedAtUnixMs: time.Now().UnixMilli(),
	}

	ids.installationID = resolveConvergedInstallationID(account, seed)
	if ids.installationID == "" {
		return nil
	}

	switch mode {
	case codexFingerprintDevice:
		return ids

	case codexFingerprintSession:
		ids.sessionID = resolveConvergedSessionID(seed)
		ids.threadID = resolveConvergedThreadID(seed, clientSessionID)
		if ids.threadID == "" {
			ids.threadID = ids.sessionID
		}
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.windowID = ids.threadID + ":0"
		return ids

	case codexFingerprintFull:
		ids.sessionID = resolveConvergedSessionID(seed)
		ids.threadID = ids.sessionID
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.windowID = ids.threadID + ":0"
		return ids
	}

	return nil
}

// extractClientSessionID 从请求头中提取客户端原始的会话标识。
// 优先取 session-id（连字符形式，Codex CLI 标准），回退到 session_id（下划线形式）。
// 返回的值尚未被 isolateOpenAISessionID 改写，是客户端的真实标识。
func extractClientSessionID(h http.Header) string {
	if v := strings.TrimSpace(h.Get("session-id")); v != "" {
		return v
	}
	return strings.TrimSpace(h.Get("session_id"))
}

// resolveCodexFingerprintIDsFromRequest 从客户端原始请求头中提取 session-id，
// 结合账号配置一次性解析收敛 ID 集合。调用方应将返回的 ids 同时传给
// applyCodexFingerprintHeaders 和 applyCodexFingerprintClientMetadata。
func resolveCodexFingerprintIDsFromRequest(account *Account, clientHeaders http.Header) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	mode := account.GetCodexFingerprintMode()
	if mode == codexFingerprintOff {
		return nil
	}
	clientSessionID := ""
	if clientHeaders != nil {
		clientSessionID = extractClientSessionID(clientHeaders)
	}
	return resolveCodexFingerprintIDs(account, clientSessionID, mode)
}

// resolveStagedCodexFingerprintIDsFromRequest derives identity from the
// credential source introduced upstream in v0.1.181, while retaining the
// selected scheduler row ID used to reject stale failover snapshots.
func resolveStagedCodexFingerprintIDsFromRequest(c *gin.Context, selected *Account, clientHeaders http.Header) *codexFingerprintIDs {
	if selected == nil {
		return nil
	}
	ids := resolveCodexFingerprintIDsFromRequest(codexAccountIdentitySource(c, selected), clientHeaders)
	if ids != nil {
		ids.accountID = selected.ID
	}
	return ids
}

// prepareCodexFingerprintSyntheticRequest 让 admin test、usage probe 和额度探测
// 使用与真实流量相同的账号指纹。scope 只区分合成会话，不含任何客户端原值。
func prepareCodexFingerprintSyntheticRequest(account *Account, scope string, reqBody map[string]any) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	ids := resolveCodexFingerprintIDs(account, "sub2api-probe:"+scope, account.GetCodexFingerprintMode())
	if ids != nil {
		applyCodexFingerprintClientMetadata(reqBody, ids)
	}
	return ids
}

// applyCodexFingerprintHeaders 按预计算的收敛 ID 改写出站 HTTP 头中的设备指纹。
// 在 buildUpstreamRequest 的白名单透传之后、enforceCodexIdentityHeaders 之前调用。
func applyCodexFingerprintHeaders(h http.Header, ids *codexFingerprintIDs) {
	if h == nil || ids == nil {
		return
	}
	sanitizeCodexFingerprintTransportHeaders(h)

	// 所有非 off 模式都收敛 installation_id
	h.Set("x-codex-installation-id", ids.installationID)

	if ids.mode == codexFingerprintDevice {
		rewriteCodexTurnMetadataFields(h, map[string]any{
			"installation_id": ids.installationID,
		})
		return
	}

	// session / full 模式：改写所有相关头
	h.Set("x-codex-window-id", ids.windowID)
	h.Set("x-client-request-id", ids.threadID)
	// 连字符形式和下划线形式都改写，保证一致
	h.Set("session-id", ids.sessionID)
	h.Set("session_id", ids.sessionID)
	h.Set("conversation_id", ids.sessionID)
	h.Set("thread-id", ids.threadID)

	rewriteCodexTurnMetadataFields(h, map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": ids.turnStartedAtUnixMs,
	})
}

// sanitizeCodexFingerprintTransportHeaders 在写入服务端身份前先清除客户端可控的
// 环境/追踪/协商通道。Accept、turn-state 不在此删除：Accept 由各协议构造器按
// SSE/JSON 明确设置；turn-state 是上游签发并由 openai_codex_turn_state.go 做
// 账号溯源校验的状态，不是可伪造的本地指纹。Live attestation 由独立服务端路径
// 生成，若意外进入 Responses 请求则在这里剥离。
func sanitizeCodexFingerprintTransportHeaders(h http.Header) {
	for _, name := range []string{
		"Cookie",
		"Accept-Language",
		"OpenAI-Beta",
		"X-Codex-Beta-Features",
		"Traceparent",
		"Tracestate",
		"Baggage",
		"X-Cloud-Trace-Context",
		"X-Amzn-Trace-Id",
		"X-Oai-Attestation",
		"X-Stainless-Timeout",
		"X-Stainless-Read-Timeout",
		"X-Stainless-Connect-Timeout",
		"X-Request-Timeout",
		"Request-Timeout",
		"Grpc-Timeout",
	} {
		h.Del(name)
	}
}

var codexTurnMetadataAllowedKeys = map[string]struct{}{
	"installation_id":         {},
	"session_id":              {},
	"thread_id":               {},
	"turn_id":                 {},
	"window_id":               {},
	"turn_started_at_unix_ms": {},
	"sandbox":                 {},
	"thread_source":           {},
	// Upstream v0.1.181 guardian affinity protocol fields.
	"parent_thread_id": {},
	"subagent_kind":    {},
}

var codexClientMetadataAllowedKeys = map[string]struct{}{
	"x-codex-installation-id": {},
	"session_id":              {},
	"thread_id":               {},
	"turn_id":                 {},
	"x-codex-window-id":       {},
	"x-codex-turn-metadata":   {},
	"ws_request_header_x_openai_internal_codex_responses_lite": {},
	"x-codex-ws-stream-request-start-ms":                       {},
}

func retainCodexMetadataKeys(metadata map[string]any, allowed map[string]struct{}) bool {
	modified := false
	for key := range metadata {
		if _, ok := allowed[key]; ok {
			continue
		}
		delete(metadata, key)
		modified = true
	}
	return modified
}

// rewriteCodexTurnMetadataFields 解析 x-codex-turn-metadata 头中的 JSON，
// 替换指定字段后回写。合法对象只保留闭集允许字段；cwd/workspace/Git/OS、
// terminal、plugin/skill/MCP、trace 及未来未知字段默认删除（fail closed）。
// 非法/非对象值重建为最小合法 metadata，避免 flat 与 embedded identity 分裂。
func rewriteCodexTurnMetadataFields(h http.Header, fields map[string]any) {
	raw := strings.TrimSpace(h.Get("x-codex-turn-metadata"))
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		metadata = make(map[string]any, len(fields))
	}
	retainCodexMetadataKeys(metadata, codexTurnMetadataAllowedKeys)
	for k, v := range fields {
		metadata[k] = v
	}
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return
	}
	h.Set("x-codex-turn-metadata", string(rebuilt))
}

// applyCodexFingerprintClientMetadata 按预计算的收敛 ID 改写请求体中的 client_metadata。
// 使用与头改写相同的 ids 实例，确保 turn_id 等随机字段一致。
func applyCodexFingerprintClientMetadata(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil || ids == nil {
		return false
	}

	captureCodexFingerprintOriginalBodySessionID(ids, reqBody["client_metadata"])
	existing, _ := reqBody["client_metadata"].(map[string]any)
	if existing == nil {
		existing = make(map[string]any)
	}

	modified := false
	if applyCodexFingerprintToClientMetadataMap(existing, ids) {
		reqBody["client_metadata"] = existing
		modified = true
	}
	if applyCodexFingerprintPromptCacheKey(reqBody, ids) {
		modified = true
	}
	return modified
}

// applyCodexFingerprintToClientMetadataMap 是 client_metadata 改写的共享核心，
// map 版（非透传，body 已解码）与 raw 字节版（透传热路径）都经由它，保证两条
// 路径的收敛语义永不漂移。
func applyCodexFingerprintToClientMetadataMap(existing map[string]any, ids *codexFingerprintIDs) bool {
	if existing == nil || ids == nil {
		return false
	}

	modified := retainCodexMetadataKeys(existing, codexClientMetadataAllowedKeys)

	if ids.installationID != "" {
		existing["x-codex-installation-id"] = ids.installationID
		modified = true
	}

	if ids.mode == codexFingerprintDevice {
		rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
			"installation_id": ids.installationID,
		})
		return modified
	}

	// session / full 模式
	existing["session_id"] = ids.sessionID
	existing["thread_id"] = ids.threadID
	existing["turn_id"] = ids.turnID
	existing["x-codex-window-id"] = ids.windowID

	rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": ids.turnStartedAtUnixMs,
	})
	return true
}

func captureCodexFingerprintOriginalBodySessionID(ids *codexFingerprintIDs, clientMetadata any) {
	if ids == nil || ids.originalBodySessionIDCaptured {
		return
	}
	ids.originalBodySessionIDCaptured = true
	if clientMetadata == nil {
		return
	}
	switch metadata := clientMetadata.(type) {
	case map[string]any:
		if sessionID, ok := metadata["session_id"].(string); ok {
			ids.originalBodySessionID = strings.TrimSpace(sessionID)
		}
	case map[string]string:
		ids.originalBodySessionID = strings.TrimSpace(metadata["session_id"])
	}
}

func captureCodexFingerprintOriginalBodySessionIDRaw(ids *codexFingerprintIDs, value gjson.Result) {
	if ids == nil || ids.originalBodySessionIDCaptured {
		return
	}
	ids.originalBodySessionIDCaptured = true
	if value.Exists() && value.Type == gjson.String {
		ids.originalBodySessionID = strings.TrimSpace(value.String())
	}
}

func shouldRewriteCodexFingerprintPromptCacheKey(ids *codexFingerprintIDs, promptCacheKey string) bool {
	if ids == nil || !ids.originalBodySessionIDCaptured || ids.originalBodySessionID == "" || ids.sessionID == "" {
		return false
	}
	if ids.mode != codexFingerprintSession && ids.mode != codexFingerprintFull {
		return false
	}
	return promptCacheKey == ids.originalBodySessionID
}

func applyCodexFingerprintPromptCacheKey(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil {
		return false
	}
	promptCacheKey, ok := reqBody["prompt_cache_key"].(string)
	if !ok || strings.TrimSpace(promptCacheKey) == "" || !shouldRewriteCodexFingerprintPromptCacheKey(ids, promptCacheKey) {
		return false
	}
	if promptCacheKey == ids.sessionID {
		return false
	}
	reqBody["prompt_cache_key"] = ids.sessionID
	return true
}

// applyCodexFingerprintClientMetadataRaw 在原始 JSON 字节上改写 client_metadata，
// 供透传路径使用——透传是热路径，禁止对可能高达数十 MB 的 body 做全量
// Unmarshal（见 forwardOpenAIPassthrough 的轻量提取注释）。实现为：gjson 提取
// client_metadata 小对象单独解码，经共享核心改写后 sjson 一次性拼回，body
// 其余字节原样保留；root prompt_cache_key 仅在可证明是 body session 默认值时
// 做标量改写。语义与 applyCodexFingerprintClientMetadata 逐点一致（含
// "非对象值整体替换为收敛集合"的行为）。
func applyCodexFingerprintClientMetadataRaw(body []byte, ids *codexFingerprintIDs) ([]byte, bool, error) {
	if len(body) == 0 || ids == nil {
		return body, false, nil
	}
	// 非 JSON 对象的 body（数组/标量/畸形）没有 client_metadata 语义，
	// sjson 在这类根上写字段会改写整体结构，直接放行保持原样。
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		captureCodexFingerprintOriginalBodySessionIDRaw(ids, gjson.Result{})
		return body, false, nil
	}

	existing := map[string]any{}
	if cm := gjson.GetBytes(body, "client_metadata"); cm.IsObject() {
		captureCodexFingerprintOriginalBodySessionIDRaw(ids, gjson.GetBytes(body, "client_metadata.session_id"))
		if err := json.Unmarshal([]byte(cm.Raw), &existing); err != nil {
			return body, false, fmt.Errorf("decode client_metadata for fingerprint: %w", err)
		}
	} else {
		captureCodexFingerprintOriginalBodySessionIDRaw(ids, gjson.Result{})
	}

	next := body
	modified := false
	if applyCodexFingerprintToClientMetadataMap(existing, ids) {
		raw, err := json.Marshal(existing)
		if err != nil {
			return body, false, fmt.Errorf("encode converged client_metadata: %w", err)
		}
		var setErr error
		next, setErr = sjson.SetRawBytes(body, "client_metadata", raw)
		if setErr != nil {
			return body, false, fmt.Errorf("splice converged client_metadata: %w", setErr)
		}
		modified = true
	}
	promptCacheKey := gjson.GetBytes(body, "prompt_cache_key")
	if promptCacheKey.Exists() && promptCacheKey.Type == gjson.String && strings.TrimSpace(promptCacheKey.String()) != "" && shouldRewriteCodexFingerprintPromptCacheKey(ids, promptCacheKey.String()) {
		rewritten, err := sjson.SetBytes(next, "prompt_cache_key", ids.sessionID)
		if err != nil {
			return body, false, fmt.Errorf("splice converged prompt_cache_key: %w", err)
		}
		next = rewritten
		modified = true
	}
	return next, modified, nil
}

// rewriteClientMetadataEmbeddedTurnMetadata 改写 client_metadata 中内嵌的
// x-codex-turn-metadata JSON 字符串里的指定字段。闭集外字段一律删除；缺失、
// 非法或非对象值会重建，避免 flat client_metadata 与 embedded metadata
// 暴露两套身份。
func rewriteClientMetadataEmbeddedTurnMetadata(clientMetadata map[string]any, fields map[string]any) {
	raw, ok := clientMetadata["x-codex-turn-metadata"].(string)
	if !ok {
		raw = ""
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		metadata = make(map[string]any, len(fields))
	}
	retainCodexMetadataKeys(metadata, codexTurnMetadataAllowedKeys)
	for k, v := range fields {
		metadata[k] = v
	}
	if rebuilt, err := json.Marshal(metadata); err == nil {
		clientMetadata["x-codex-turn-metadata"] = string(rebuilt)
	}
}
