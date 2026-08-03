import {useState} from 'react';

type Section = 'overview' | 'sessions' | 'tasks' | 'settings';

const sections: Array<{id: Section; label: string; icon: string}> = [
  {id: 'overview', label: '概览', icon: '⌂'},
  {id: 'sessions', label: '会话', icon: '▤'},
  {id: 'tasks', label: '实时任务', icon: '◉'},
  {id: 'settings', label: '设置', icon: '⚙'},
];

export function App() {
  const [section, setSection] = useState<Section>('overview');

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
        <div className="sidebar-status"><span className="status-dot" />已连接</div>
      </aside>
      <main className="content">{renderSection(section)}</main>
    </div>
  );
}

function renderSection(section: Section) {
  switch (section) {
    case 'sessions': return <Sessions />;
    case 'tasks': return <Tasks />;
    case 'settings': return <Settings />;
    default: return <Overview />;
  }
}

function Overview() {
  return (
    <>
      <PageHeader title="概览" description="保持电脑在线，即可从手机继续 Codex 会话。" />
      <section className="device-card">
        <div className="icon-tile">▣</div>
        <div className="grow"><span className="eyebrow">当前设备</span><h2>Workstation</h2><p><span className="status-dot" />Linux · 在线</p></div>
        <button className="secondary" type="button">设备设置</button>
      </section>
      <div className="metric-grid">
        <Metric label="可发现会话" value="12" />
        <Metric label="运行任务" value="1" />
        <Metric label="今日同步" value="8" />
      </div>
      <section className="panel">
        <div className="panel-heading"><h3>最近事件</h3><button type="button" className="link-button">刷新</button></div>
        <Event title="会话列表已同步" meta="刚刚 · 12 个会话" />
        <Event title="任务已完成" meta="10:18 · 修复支付回调" />
        <Event title="设备重新连接" meta="09:42 · 网络已恢复" />
      </section>
    </>
  );
}

function Sessions() {
  return (
    <>
      <PageHeader title="会话" description="只同步脱敏后的会话元数据与所选消息。" />
      <div className="toolbar"><input aria-label="搜索会话" placeholder="搜索会话" /><button className="primary" type="button">同步会话</button></div>
      <section className="panel session-list">
        <Session title="修复支付回调" path="~/works/sub2api" state="可继续" />
        <Session title="更新登录流程" path="~/works/sub2api" state="可继续" />
        <Session title="整理 API 文档" path="~/works/docs" state="只读" />
      </section>
    </>
  );
}

function Tasks() {
  return (
    <>
      <PageHeader title="实时任务" description="显示从手机发送到本机 Codex 的执行状态。" />
      <section className="panel">
        <Task title="补上失败重试，并更新相关测试" session="修复支付回调" state="运行中" active />
        <Task title="检查登录模块" session="更新登录流程" state="已完成" />
      </section>
    </>
  );
}

function Settings() {
  return (
    <>
      <PageHeader title="设置" description="账号、设备与本机 Codex 安全策略。" />
      <section className="panel settings-list">
        <Setting label="Sub2API 站点" value="https://••••.example" />
        <Setting label="登录账号" value="admin@••••.com" />
        <Setting label="Codex CLI" value="已发现" />
        <Setting label="同步隐私" value="路径脱敏" />
        <Setting label="本机安全策略" value="非交互 · 不自动扩大权限" />
      </section>
    </>
  );
}

function PageHeader({title, description}: {title: string; description: string}) {
  return <header className="page-header"><div><h1>{title}</h1><p>{description}</p></div></header>;
}

function Metric({label, value}: {label: string; value: string}) {
  return <article className="metric"><span>{label}</span><strong>{value}</strong></article>;
}

function Event({title, meta}: {title: string; meta: string}) {
  return <div className="row"><span className="event-icon">✓</span><div className="grow"><strong>{title}</strong><small>{meta}</small></div></div>;
}

function Session({title, path, state}: {title: string; path: string; state: string}) {
  return <div className="row"><span className="event-icon">▤</span><div className="grow"><strong>{title}</strong><small>{path}</small></div><span className="tag">{state}</span><span>›</span></div>;
}

function Task({title, session, state, active = false}: {title: string; session: string; state: string; active?: boolean}) {
  return <div className="row"><span className={active ? 'pulse' : 'event-icon'}>{active ? '' : '✓'}</span><div className="grow"><strong>{title}</strong><small>{session}</small></div><span className={active ? 'tag blue' : 'tag'}>{state}</span></div>;
}

function Setting({label, value}: {label: string; value: string}) {
  return <button type="button" className="setting"><span>{label}</span><span>{value}　›</span></button>;
}
