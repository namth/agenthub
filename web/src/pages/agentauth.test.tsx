// Test nghiệm thu TASK-034 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-auth.md §4, §6; DEC-24, DEC-29, DEC-30; TC-67.
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import Fleet from './Fleet';
import MyMachines from './MyMachines';
import Settings from './Settings';

type Call = { url: string; method: string; body?: unknown };
let calls: Call[] = [];
let routes: Record<string, () => { status: number; body: unknown }> = {};

function ok(data: unknown, status = 200) {
  return { status, body: { success: true, data, error: null } };
}
function fail(status: number, code: string, message = code) {
  return { status, body: { success: false, data: null, error: { code, message } } };
}

beforeEach(() => {
  calls = [];
  routes = {};
  vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ url: input, method, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const h = routes[`${method} ${input.split('?')[0]}`];
    if (!h) return new Response(JSON.stringify(fail(404, 'NOT_FOUND').body), { status: 404 });
    const r = h();
    return new Response(JSON.stringify(r.body), { status: r.status, headers: { 'Content-Type': 'application/json' } });
  });
  vi.stubGlobal('confirm', () => true);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const wrap = (ui: React.ReactNode, path: string) => (
  <MemoryRouter initialEntries={[path]}>
    <Routes>
      <Route path="/fleet" element={ui} />
      <Route path="/my/machines" element={ui} />
      <Route path="/settings" element={ui} />
    </Routes>
  </MemoryRouter>
);

const agent = (over: Record<string, unknown>) => ({
  kind: 'CLAUDE', status: 'IDLE', cooldown_until: null, lead_priority: null, soft_max_jobs: null, soft_window_hours: 5,
  jobs_in_window: 0, current_job: null, lead_eligible: true, auth_status: 'OK', auth_detail: null, auth_checked_at: null, projects: null,
  ...over,
});

const fleet = {
  lead: { state: 'HELD', agent_id: 'lead/claude', epoch: 3, expires_at: '2026-11-04T10:01:30Z', vacant_since: null, eta: null, waiting_jobs: 0 },
  workers: [
    { id: 'imac', name: 'imac', os: 'darwin', conn_status: 'ONLINE', offline_since: null, gh_auth: 'OK', owner: null, version: 'v1', paused: false, agents: [
      agent({ id: 'imac/claude', auth_status: 'NEEDS_AUTH', auth_detail: 'claude: loggedIn=false' }),
      agent({ id: 'imac/antigravity', kind: 'ANTIGRAVITY', auth_status: 'UNKNOWN', lead_eligible: true }),
    ] },
    { id: 'lead', name: 'lead', os: 'darwin', conn_status: 'ONLINE', offline_since: null, gh_auth: 'OK', owner: null, version: 'v1', paused: false, agents: [
      agent({ id: 'lead/claude', status: 'BUSY' }),
      agent({ id: 'lead/antigravity', kind: 'ANTIGRAVITY', lead_eligible: false }),
    ] },
    { id: 'box', name: 'box', os: 'linux', conn_status: 'ONLINE', offline_since: null, gh_auth: 'NEEDS_AUTH', owner: null, version: 'v1', paused: false, agents: [
      agent({ id: 'box/claude' }),
    ] },
  ],
};

describe('SCR-05 trạng thái đăng nhập', () => {
  it('nhãn Cần đăng nhập / Chưa kiểm được / không nhãn khi OK, kèm lệnh gợi ý', async () => {
    routes['GET /api/fleet'] = () => ok(fleet);
    render(wrap(<Fleet />, '/fleet'));
    const needs = await screen.findByTestId('agent-imac/claude');
    expect(within(needs).getByText('Cần đăng nhập')).toBeTruthy();
    expect(needs.textContent).toMatch(/\/login/);
    expect(within(screen.getByTestId('agent-imac/antigravity')).getByText('Chưa kiểm được')).toBeTruthy();
    const okRow = screen.getByTestId('agent-lead/claude');
    expect(within(okRow).queryByText('Cần đăng nhập')).toBeNull();
    expect(within(okRow).queryByText('Chưa kiểm được')).toBeNull();
  });

  it('băng GitHub CLI chưa đăng nhập ở máy gh_auth=NEEDS_AUTH', async () => {
    routes['GET /api/fleet'] = () => ok(fleet);
    render(wrap(<Fleet />, '/fleet'));
    const box = await screen.findByTestId('worker-box');
    const banner = within(box).getByRole('status');
    expect(banner.textContent).toMatch(/GitHub CLI chưa đăng nhập/);
    expect(banner.textContent).toMatch(/gh auth login/);
    expect(within(screen.getByTestId('worker-imac')).queryByText(/GitHub CLI chưa đăng nhập/)).toBeNull();
  });

  it('Trao vai: mọi loại agent đủ điều kiện; loại agent NEEDS_AUTH, lead_eligible=false, máy gh NEEDS_AUTH', async () => {
    routes['GET /api/fleet'] = () => ok(fleet);
    render(wrap(<Fleet />, '/fleet'));
    await screen.findByTestId('agent-imac/antigravity');
    const btns = screen.getAllByRole('button', { name: 'Trao vai' });
    expect(btns.length).toBe(1);
    expect(within(screen.getByTestId('agent-imac/antigravity')).getByRole('button', { name: 'Trao vai' })).toBeTruthy();
  });

  it('công tắc Cho làm trưởng phòng hiện cho cả agent Antigravity', async () => {
    routes['GET /api/fleet'] = () => ok(fleet);
    routes['PATCH /api/admin/agents/lead%2Fantigravity'] = () => ok({});
    render(wrap(<Fleet />, '/fleet'));
    const row = await screen.findByTestId('agent-lead/antigravity');
    const sw = within(row).getByLabelText('Cho làm trưởng phòng') as HTMLInputElement;
    expect(sw.checked).toBe(false);
    fireEvent.click(sw);
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ lead_eligible: true }));
  });

  it('Đã đăng nhập, kiểm lại → POST auth-recheck với agent_id; băng gh gửi {}', async () => {
    routes['GET /api/fleet'] = () => ok(fleet);
    routes['POST /api/my/machines/imac/auth-recheck'] = () => ({ status: 202, body: { success: true, data: null, error: null } });
    routes['POST /api/my/machines/box/auth-recheck'] = () => ({ status: 202, body: { success: true, data: null, error: null } });
    render(wrap(<Fleet />, '/fleet'));
    fireEvent.click(within(await screen.findByTestId('agent-imac/claude')).getByRole('button', { name: /Đã đăng nhập, kiểm lại/ }));
    await waitFor(() => expect(calls.some((c) => c.url === '/api/my/machines/imac/auth-recheck')).toBe(true));
    expect(calls.find((c) => c.url === '/api/my/machines/imac/auth-recheck')!.body).toEqual({ agent_id: 'imac/claude' });
    expect(await screen.findByText(/Đã gửi yêu cầu kiểm lại/)).toBeTruthy();
    fireEvent.click(within(screen.getByTestId('worker-box')).getByRole('button', { name: /Đã đăng nhập, kiểm lại/ }));
    await waitFor(() => expect(calls.some((c) => c.url === '/api/my/machines/box/auth-recheck')).toBe(true));
    expect(calls.find((c) => c.url === '/api/my/machines/box/auth-recheck')!.body).toEqual({});
    expect(within(screen.getByTestId('agent-imac/antigravity')).queryByRole('button', { name: /kiểm lại/ })).toBeNull();
  });

  it('kiểm lại: đang gửi thì khoá nút; 409 → Máy đang offline', async () => {
    routes['GET /api/fleet'] = () => ok(fleet);
    let release!: () => void;
    const gate = new Promise<void>((r) => { release = r; });
    vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
      const method = (init?.method ?? 'GET').toUpperCase();
      calls.push({ url: input, method, body: init?.body ? JSON.parse(String(init.body)) : undefined });
      if (method === 'POST') {
        await gate;
        return new Response(JSON.stringify(fail(409, 'WORKER_OFFLINE').body), { status: 409, headers: { 'Content-Type': 'application/json' } });
      }
      return new Response(JSON.stringify(ok(fleet).body), { status: 200, headers: { 'Content-Type': 'application/json' } });
    });
    render(wrap(<Fleet />, '/fleet'));
    const row = await screen.findByTestId('agent-imac/claude');
    fireEvent.click(within(row).getByRole('button', { name: /Đã đăng nhập, kiểm lại/ }));
    const busy = await within(row).findByRole('button', { name: /Đang kiểm/ });
    expect((busy as HTMLButtonElement).disabled).toBe(true);
    await act(async () => { release(); });
    expect((await screen.findByRole('alert')).textContent).toMatch(/Máy đang offline/);
  });

  it('dữ liệu cũ không có trường auth → không nhãn đỏ, không băng gh', async () => {
    const legacy = JSON.parse(JSON.stringify(fleet));
    for (const w of legacy.workers) {
      delete w.gh_auth;
      for (const a of w.agents) { delete a.auth_status; delete a.auth_detail; }
    }
    routes['GET /api/fleet'] = () => ok(legacy);
    render(wrap(<Fleet />, '/fleet'));
    await screen.findByTestId('agent-imac/claude');
    expect(screen.queryByText('Cần đăng nhập')).toBeNull();
    expect(screen.queryByText(/GitHub CLI chưa đăng nhập/)).toBeNull();
  });
});

describe('SCR-11 trạng thái đăng nhập', () => {
  const mine = [{
    id: 'imac', name: 'imac', os: 'darwin', conn_status: 'ONLINE', version: 'v1', paused: false, gh_auth: 'NEEDS_AUTH',
    agents: [
      { id: 'imac/claude', kind: 'CLAUDE', status: 'IDLE', projects: [], current_job: null, auth_status: 'NEEDS_AUTH', auth_detail: 'claude: loggedIn=false', auth_checked_at: null },
      { id: 'imac/antigravity', kind: 'ANTIGRAVITY', status: 'IDLE', projects: [], current_job: null, auth_status: 'NEEDS_AUTH', auth_detail: null, auth_checked_at: null },
    ],
  }];
  beforeEach(() => {
    routes['GET /api/projects'] = () => ok([]);
    routes['GET /api/my/machines'] = () => ok(mine);
  });

  it('nhãn, lệnh gợi ý theo loại agent, băng gh và kiểm lại', async () => {
    routes['POST /api/my/machines/imac/auth-recheck'] = () => ({ status: 202, body: { success: true, data: null, error: null } });
    render(wrap(<MyMachines />, '/my/machines'));
    const m = await screen.findByTestId('machine-imac');
    expect(within(m).getAllByText('Cần đăng nhập').length).toBe(2);
    expect(m.textContent).toMatch(/\/login/);
    expect(m.textContent).toMatch(/agy/);
    expect(within(m).getByRole('status').textContent).toMatch(/gh auth login/);
    const btns = within(m).getAllByRole('button', { name: /Đã đăng nhập, kiểm lại/ });
    expect(btns.length).toBe(3); // 2 agent + băng gh
    fireEvent.click(btns[0]);
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/my/machines/imac/auth-recheck')).toBe(true));
  });
});

describe('SCR-07 dòng Telegram', () => {
  it.each([
    [true, /Telegram: đang bật/],
    [false, /Telegram: đang tắt/],
  ])('telegram_enabled=%s', async (on, re) => {
    routes['GET /api/admin/settings'] = () => ok({ office_enabled: false, telegram_enabled: on });
    render(wrap(<Settings onOfficeChange={() => {}} />, '/settings'));
    expect(await screen.findByText(re)).toBeTruthy();
    expect(screen.getByText(re).textContent).toMatch(/\.env/);
    expect(screen.queryByLabelText(/token/i)).toBeNull();
  });
});
