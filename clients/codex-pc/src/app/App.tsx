import {invoke} from '@tauri-apps/api/core';
import {FormEvent, useEffect, useRef, useState} from 'react';

type Section = 'overview' | 'sessions' | 'tasks' | 'settings';

type PublicSession = {
  site_url: string;
  user_id: number;
  email: string;
  role: string;
  expires_at_epoch_seconds: number;
};

type AuthStatus = {
  authenticated: boolean;
  requires_two_factor: boolean;
  session: PublicSession | null;
};

type LoginResult =
  | {status: 'authenticated'; session: PublicSession}
  | {status: 'requires_two_factor'; email_masked: string};

type CodexThread = {
  id: string;
  title: string;
  cwdLabel: string | null;
  status: string;
  canWrite: boolean;
  updatedAt: number | null;
};

type ThreadPage = {
  data: CodexThread[];
  nextCursor: string | null;
};

type RelayStatus = {
  state: string;
  device_id: string | null;
  last_error: string | null;
};

const EMAIL_KEY = 'codexPc.email';

const sections: Array<{id: Section; label: string; icon: string}> = [
  {id: 'overview', label: '概览', icon: '⌂'},
  {id: 'sessions', label: '会话', icon: '▤'},
  {id: 'tasks', label: '实时任务', icon: '◉'},
  {id: 'settings', label: '设置', icon: '⚙'},
];

export function App() {
  const bootstrapped = useRef(false);
  const [session, setSession] = useState<PublicSession | null>(null);
  const [loading, setLoading] = useState(true);
  const [section, setSection] = useState<Section>('overview');
  const [threads, setThreads] = useState<CodexThread[]>([]);
  const [codexReady, setCodexReady] = useState(false);
  const [deviceRegistered, setDeviceRegistered] = useState(false);
  const [relayState, setRelayState] = useState('disconnected');
  const [codexError, setCodexError] = useState('');

  useEffect(() => {
    if (bootstrapped.current) return;
    bootstrapped.current = true;
    void bootstrapSession().then(setSession).finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    if (!session) return;
    void invoke('codex_start')
      .then(async () => {
        setCodexReady(true);
        try {
          const status = await invoke<RelayStatus>('collaboration_connect');
          setDeviceRegistered(Boolean(status.device_id));
          setRelayState(status.state);
        } catch (error) {
          setRelayState('error');
          setCodexError(errorMessage(error));
        }
      })
      .catch((error) => setCodexError(errorMessage(error)));
  }, [session]);

  useEffect(() => {
    if (!session) return;
    const timer = window.setInterval(() => {
      void invoke<RelayStatus>('collaboration_status').then((status) => {
        setDeviceRegistered(Boolean(status.device_id));
        setRelayState((current) =>
          current === 'error' && status.state === 'disconnected'
            ? current
            : status.state,
        );
        if (status.last_error && status.state !== 'connected') {
          setCodexError(errorMessage(status.last_error));
        }
      });
    }, 2000);
    return () => window.clearInterval(timer);
  }, [session]);

  async function refreshThreads(searchTerm = '') {
    setCodexError('');
    try {
      const page = await invoke<ThreadPage>('codex_list_threads', {
        limit: 50,
        cursor: null,
        searchTerm: searchTerm.trim() || null,
        archived: false,
      });
      setThreads(page.data);
    } catch (error) {
      setCodexError(errorMessage(error));
    }
  }

  if (loading) return <LoadingScreen />;
  if (!session) return <LoginScreen onAuthenticated={setSession} />;

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">C</span>
          <div><strong>Codex PC</strong><small>Sub2API Companion</small></div>
        </div>
        <nav aria-label="主导航">
          {sections.map((item) => (
            <button
              type="button"
              key={item.id}
              className={section === item.id ? 'nav-item active' : 'nav-item'}
              onClick={() => setSection(item.id)}
            >
              <span aria-hidden="true">{item.icon}</span>{item.label}
            </button>
          ))}
        </nav>
        <div className="sidebar-account">
          <strong>{maskEmail(session.email)}</strong>
          <small>{siteHost(session.site_url)}</small>
        </div>
        <div className="sidebar-status">
          <span className={relayState === 'connected' ? 'status-dot' : 'status-dot muted'} />
          {relayStatusLabel(relayState, codexReady)}
        </div>
      </aside>
      <main className="content">
        {renderSection({
          section,
          session,
          threads,
          codexReady,
          deviceRegistered,
          relayState,
          codexError,
          refreshThreads,
          onLogout: () => setSession(null),
        })}
      </main>
    </div>
  );
}

async function bootstrapSession(): Promise<PublicSession | null> {
  try {
    const status = await invoke<AuthStatus>('panel_auth_status');
    if (status.authenticated && status.session) return status.session;
    const email = localStorage.getItem(EMAIL_KEY);
    if (!email) return null;
    return await invoke<PublicSession>('panel_restore_session', {email});
  } catch {
    return null;
  }
}

function renderSection(props: {
  section: Section;
  session: PublicSession;
  threads: CodexThread[];
  codexReady: boolean;
  deviceRegistered: boolean;
  relayState: string;
  codexError: string;
  refreshThreads: (searchTerm?: string) => Promise<void>;
  onLogout: () => void;
}) {
  switch (props.section) {
    case 'sessions':
      return <Sessions threads={props.threads} error={props.codexError} onRefresh={props.refreshThreads} />;
    case 'tasks':
      return <Tasks />;
    case 'settings':
      return <Settings session={props.session} codexReady={props.codexReady} deviceRegistered={props.deviceRegistered} relayState={props.relayState} onLogout={props.onLogout} />;
    default:
      return <Overview threads={props.threads} codexReady={props.codexReady} deviceRegistered={props.deviceRegistered} relayState={props.relayState} error={props.codexError} onRefresh={props.refreshThreads} />;
  }
}

function LoginScreen({onAuthenticated}: {onAuthenticated: (session: PublicSession) => void}) {
  const [email, setEmail] = useState(localStorage.getItem(EMAIL_KEY) ?? '');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [emailMasked, setEmailMasked] = useState('');
  const [step, setStep] = useState<'login' | 'twoFactor'>('login');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    setError('');
    try {
      if (step === 'login') {
        const result = await invoke<LoginResult>('panel_login', {
          email,
          password,
          turnstileToken: null,
        });
        setPassword('');
        if (result.status === 'requires_two_factor') {
          setEmailMasked(result.email_masked);
          setStep('twoFactor');
          return;
        }
        rememberIdentity(result.session);
        onAuthenticated(result.session);
        return;
      }
      const session = await invoke<PublicSession>('panel_complete_two_factor', {code});
      setCode('');
      rememberIdentity(session);
      onAuthenticated(session);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="auth-shell">
      <section className="auth-card">
        <div className="auth-brand"><span className="brand-mark">C</span><strong>Codex PC</strong></div>
        <h1>{step === 'login' ? '登录 Sub2API' : '两步验证'}</h1>
        <p>{step === 'login' ? '登录同一账号后，手机才能发现这台电脑。' : `请输入发送至 ${emailMasked || '你的验证器'} 的 6 位验证码。`}</p>
        <form onSubmit={submit}>
          {step === 'login' ? (
            <>
              <label>邮箱<input value={email} onChange={(event) => setEmail(event.target.value)} type="email" autoComplete="username" required autoFocus /></label>
              <label>密码<input value={password} onChange={(event) => setPassword(event.target.value)} type="password" autoComplete="current-password" required /></label>
            </>
          ) : (
            <label>验证码<input className="code-input" value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, '').slice(0, 6))} inputMode="numeric" autoComplete="one-time-code" maxLength={6} required autoFocus /></label>
          )}
          {error && <div className="error-banner" role="alert">{error}</div>}
          <button className="primary full" type="submit" disabled={submitting}>{submitting ? '请稍候…' : step === 'login' ? '登录' : '验证'}</button>
          {step === 'twoFactor' && <button className="text-button" type="button" onClick={() => { setStep('login'); setCode(''); setError(''); }}>返回登录</button>}
        </form>
        <small className="security-note">密码不会保存；Refresh Token 仅存入系统安全凭据库。</small>
      </section>
    </main>
  );
}

function LoadingScreen() {
  return <main className="auth-shell"><div className="loading-mark"><span className="brand-mark">C</span><span>正在恢复安全会话…</span></div></main>;
}

function Overview({threads, codexReady, deviceRegistered, relayState, error, onRefresh}: {threads: CodexThread[]; codexReady: boolean; deviceRegistered: boolean; relayState: string; error: string; onRefresh: () => Promise<void>}) {
  return (
    <>
      <PageHeader title="概览" description="保持电脑在线，即可从手机继续 Codex 会话。" />
      <section className="device-card">
        <div className="icon-tile">▣</div>
        <div className="grow"><span className="eyebrow">当前设备</span><h2>这台电脑</h2><p><span className={codexReady ? 'status-dot' : 'status-dot muted'} />{codexReady ? 'Codex CLI 已就绪' : '正在连接 Codex CLI'}</p></div>
        <button className="secondary" type="button" onClick={() => void onRefresh()} disabled={!codexReady}>检查会话</button>
      </section>
      {error && <div className="error-banner spaced" role="alert">{error}</div>}
      <div className="metric-grid">
        <Metric label="已发现会话" value={String(threads.length)} />
        <Metric label="手机任务" value="0" />
        <Metric label="设备状态" value={relayState === 'connected' ? '在线' : deviceRegistered ? '已注册' : '等待'} compact />
      </div>
      <section className="panel empty-panel"><span className="event-icon">✓</span><div><strong>无需电脑确认</strong><p>手机发送任务后将直接转交所选 Codex 会话。</p></div></section>
    </>
  );
}

function Sessions({threads, error, onRefresh}: {threads: CodexThread[]; error: string; onRefresh: (search?: string) => Promise<void>}) {
  const [search, setSearch] = useState('');
  return (
    <>
      <PageHeader title="会话" description="仅展示脱敏后的会话名称、状态和路径末级。" />
      <form className="toolbar" onSubmit={(event) => { event.preventDefault(); void onRefresh(search); }}>
        <input aria-label="搜索会话" placeholder="搜索会话" value={search} onChange={(event) => setSearch(event.target.value)} />
        <button className="primary" type="submit">查询</button>
      </form>
      {error && <div className="error-banner spaced" role="alert">{error}</div>}
      <section className="panel session-list">
        {threads.length === 0 ? <Empty title="暂无会话" description="点击查询，从本机 Codex 获取会话列表。" /> : threads.map((thread) => (
          <Session key={thread.id} title={thread.title} path={thread.cwdLabel ?? '路径已隐藏'} state={thread.canWrite ? '可继续' : '只读'} />
        ))}
      </section>
    </>
  );
}

function Tasks() {
  return (
    <>
      <PageHeader title="实时任务" description="手机发送的任务会在这里显示执行状态。" />
      <section className="panel"><Empty title="等待手机任务" description="任务无需在电脑端审批或确认。" /></section>
    </>
  );
}

function Settings({session, codexReady, deviceRegistered, relayState, onLogout}: {session: PublicSession; codexReady: boolean; deviceRegistered: boolean; relayState: string; onLogout: () => void}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function logout() {
    setBusy(true);
    setError('');
    try {
      await invoke('panel_logout');
      localStorage.removeItem(EMAIL_KEY);
      onLogout();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeader title="设置" description="账号、设备与本机 Codex 安全策略。" />
      <section className="panel settings-list">
        <Setting label="Sub2API 站点" value={siteHost(session.site_url)} />
        <Setting label="登录账号" value={maskEmail(session.email)} />
        <Setting label="账号角色" value={session.role === 'admin' ? '管理员' : '用户'} />
        <Setting label="Codex CLI" value={codexReady ? '已发现' : '未连接'} />
        <Setting label="协同设备" value={relayState === 'connected' ? '在线' : deviceRegistered ? '已注册' : '未注册'} />
        <Setting label="同步隐私" value="路径脱敏" />
        <Setting label="本机安全策略" value="非交互 · 不扩大权限" />
      </section>
      {error && <div className="error-banner spaced" role="alert">{error}</div>}
      <button className="danger-button" type="button" disabled={busy} onClick={() => void logout()}>{busy ? '正在退出…' : '退出登录'}</button>
    </>
  );
}

function PageHeader({title, description}: {title: string; description: string}) {
  return <header className="page-header"><div><h1>{title}</h1><p>{description}</p></div></header>;
}

function Metric({label, value, compact = false}: {label: string; value: string; compact?: boolean}) {
  return <article className="metric"><span>{label}</span><strong className={compact ? 'compact' : ''}>{value}</strong></article>;
}

function Session({title, path, state}: {title: string; path: string; state: string}) {
  return <div className="row"><span className="event-icon">▤</span><div className="grow"><strong>{title}</strong><small>{path}</small></div><span className="tag">{state}</span></div>;
}

function Setting({label, value}: {label: string; value: string}) {
  return <div className="setting"><span>{label}</span><span>{value}</span></div>;
}

function Empty({title, description}: {title: string; description: string}) {
  return <div className="empty"><span className="event-icon">·</span><strong>{title}</strong><p>{description}</p></div>;
}

function rememberIdentity(session: PublicSession) {
  localStorage.setItem(EMAIL_KEY, session.email);
}

function maskEmail(email: string) {
  const [name, domain] = email.split('@');
  if (!domain) return email;
  return `${name.slice(0, 2)}•••@${domain}`;
}

function siteHost(siteUrl: string) {
  try {
    return new URL(siteUrl).host;
  } catch {
    return 'Sub2API';
  }
}

function relayStatusLabel(relayState: string, codexReady: boolean) {
  if (relayState === 'connected') return '协同已连接';
  if (relayState === 'error' || relayState === 'revoked') return '协同连接失败';
  if (relayState === 'reconnecting' || relayState === 'refreshing') return '协同重连中';
  return codexReady ? '协同连接中' : '正在连接 Codex';
}

function errorMessage(reason: unknown) {
  const code = typeof reason === 'string' ? reason : String(reason);
  const messages: Record<string, string> = {
    PANEL_INVALID_SITE: '请输入有效的 Sub2API 站点地址。',
    PANEL_INSECURE_SITE: '远程站点必须使用 HTTPS。',
    PANEL_INVALID_EMAIL: '请输入有效邮箱。',
    PANEL_INVALID_PASSWORD: '请输入密码。',
    PANEL_INVALID_TWO_FACTOR_CODE: '请输入 6 位验证码。',
    PANEL_NO_PENDING_TWO_FACTOR: '验证已失效，请重新登录。',
    PANEL_SESSION_NOT_FOUND: '本机没有可恢复的登录会话。',
    PANEL_UNAUTHORIZED: '邮箱、密码或验证码不正确。',
    PANEL_FORBIDDEN: '当前账号无权登录。',
    PANEL_RATE_LIMITED: '请求过于频繁，请稍后再试。',
    PANEL_NETWORK_ERROR: '无法连接 Sub2API 站点，请检查网络和地址。',
    PANEL_SERVER_ERROR: 'Sub2API 站点暂时不可用。',
    SECURE_STORE_UNAVAILABLE: '系统安全凭据库不可用。',
    COLLAB_UNAUTHORIZED: '登录已过期，请重新登录。',
    COLLAB_FORBIDDEN: '当前账号无法注册协同设备。',
    COLLABORATION_DISABLED: '服务端尚未启用 Codex 协同，请开启 COLLABORATION_ENABLED 后重启服务。',
    COLLAB_NETWORK_ERROR: '无法连接协同服务，请检查网络。',
    COLLAB_CONNECT_FAILED: '协同连接失败，正在自动重试。',
    COLLAB_DISCONNECTED: '协同连接已断开，正在自动重试。',
    COLLAB_DEVICE_REVOKED: '这台设备已被撤销，请重新登录注册。',
    COLLAB_PROTOCOL_ERROR: '协同协议不兼容，请升级应用。',
    COLLAB_INSTALLATION_ID_CORRUPT: '本机安装身份损坏，请重新安装或清理系统凭据。',
    CODEX_NOT_FOUND: '未找到 Codex CLI，请先安装并确保命令可用。',
    CODEX_INCOMPATIBLE: '当前 Codex CLI 版本不兼容，请升级后重试。',
    CODEX_TIMEOUT: 'Codex 响应超时，请重试。',
  };
  return messages[code] ?? '操作失败，请重试。';
}
