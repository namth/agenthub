// Test FEAT-11 — do trưởng phòng viết.
// Đặc tả: docs/features/agent-accounts.md §4, §6; DEC-41 → DEC-43; TC-103.
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import Fleet from './Fleet';
import MyMachines from './MyMachines';

type Call = { url: string; method: string };
let calls: Call[] = [];
let routes: Record<string, () => { status: number; body: unknown }> = {};
const ok = (data: unknown, status = 200) => ({ status, body: { success: true, data, error: null } });
const fail = (status: number, code: string, message = code) => ({ status, body: { success: false, data: null, error: { code, message } } });

beforeEach(() => {
  calls = [];
  routes = {};
  vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ url: input, method });
    const h = routes[`${method} ${input.split('?')[0]}`];
    const r = h ? h() : fail(404, 'NOT_FOUND');
    return new Response(JSON.stringify(r.body), { status: r.status, headers: { 'Content-Type': 'application/json' } });
  });
  vi.stubGlobal('confirm', () => true);
  routes['GET /api/projects'] = () => ok([]);
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const wrap = (ui: React.ReactNode, path: string) => (
  <MemoryRouter initialEntries={[path]}>
    <Routes>
      <Route path="/fleet" element={ui} />
      <Route path="/my/machines" element={ui} />
    </Routes>
  </MemoryRouter>
);

const agent = (over: Record<string, unknown>) => ({
  kind: 'CLAUDE', status: 'IDLE', cooldown_until: null, lead_priority: null, soft_max_jobs: null, soft_window_hours: 5,
  jobs_in_window: 0, current_job: null, lead_eligible: true, auth_status: 'OK', auth_detail: null, auth_checked_at: null, projects: null,
  ...over,
});
const fleetWith = (agents: unknown[]) => ({
  lead: { state: 'VACANT', agent_id: null, epoch: 0, expires_at: null, vacant_since: null, eta: null, waiting_jobs: 0 },
  workers: [{ id: 'imac', name: 'imac', os: 'darwin', conn_status: 'ONLINE', offline_since: null, gh_auth: 'OK', owner: null, version: 'v1', paused: false, agents }],
});

describe('SCR-05 Cụm máy — tài khoản agent (FEAT-11)', () => {
  it('hiện tên tài khoản; NEEDS_AUTH gợi ý `agenthub account login <name>`', async () => {
    routes['GET /api/fleet'] = () => ok(fleetWith([
      agent({ id: 'imac/claude', name: 'claude' }),
      agent({ id: 'imac/claude-2', name: 'claude-2', auth_status: 'NEEDS_AUTH', auth_detail: 'OAuth session expired' }),
    ]));
    render(wrap(<Fleet />, '/fleet'));
    const row = await screen.findByTestId('agent-imac/claude-2');
    expect(row.textContent).toMatch(/Cần đăng nhập/);
    expect(row.textContent).toMatch(/agenthub account login claude-2/);
  });

  it('Xoá: xác nhận → DELETE /api/admin/agents/{id} → dòng biến mất; agent BUSY không xoá được', async () => {
    let list = [agent({ id: 'imac/claude', name: 'claude', status: 'BUSY' }), agent({ id: 'imac/claude-2', name: 'claude-2' })];
    routes['GET /api/fleet'] = () => ok(fleetWith(list));
    routes['DELETE /api/admin/agents/imac%2Fclaude-2'] = () => { list = list.slice(0, 1); return ok({ removed: 'imac/claude-2' }); };
    render(wrap(<Fleet />, '/fleet'));
    const busy = await screen.findByTestId('agent-imac/claude');
    expect((within(busy).getByRole('button', { name: 'Xoá' }) as HTMLButtonElement).disabled).toBe(true);
    const row = screen.getByTestId('agent-imac/claude-2');
    fireEvent.click(within(row).getByRole('button', { name: 'Xoá' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url === '/api/admin/agents/imac%2Fclaude-2')).toBe(true));
    await waitFor(() => expect(screen.queryByTestId('agent-imac/claude-2')).toBeNull());
  });

  it('Xoá bị huỷ ở hộp xác nhận → không gọi API; 409 → báo lỗi', async () => {
    routes['GET /api/fleet'] = () => ok(fleetWith([agent({ id: 'imac/claude-2', name: 'claude-2' })]));
    routes['DELETE /api/admin/agents/imac%2Fclaude-2'] = () => fail(409, 'CONFLICT', 'agent đang chạy việc');
    render(wrap(<Fleet />, '/fleet'));
    const row = await screen.findByTestId('agent-imac/claude-2');
    vi.stubGlobal('confirm', () => false);
    fireEvent.click(within(row).getByRole('button', { name: 'Xoá' }));
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false);
    vi.stubGlobal('confirm', () => true);
    fireEvent.click(within(row).getByRole('button', { name: 'Xoá' }));
    expect((await screen.findByRole('alert')).textContent).toMatch(/đang chạy việc/);
  });
});

const machine = (agents: unknown[]) => ({ id: 'mac-moi', name: 'mac-moi', os: 'darwin', conn_status: 'ONLINE', version: 'v1', paused: false, agents });

describe('SCR-11 Máy của tôi — tài khoản agent (FEAT-11)', () => {
  it('máy chưa có tài khoản → dòng hướng dẫn `agenthub account add`', async () => {
    routes['GET /api/my/machines'] = () => ok([machine([])]);
    render(wrap(<MyMachines />, '/my/machines'));
    expect(await screen.findByText(/Chưa có tài khoản agent/)).toBeTruthy();
    expect(screen.getByText(/agenthub account add claude/)).toBeTruthy();
  });

  it('chủ máy xoá tài khoản → DELETE /api/my/machines/{id}/agents/{name}', async () => {
    let agents = [
      { id: 'mac-moi/claude', name: 'claude', kind: 'CLAUDE', status: 'IDLE', projects: [], current_job: null, auth_status: 'OK' },
      { id: 'mac-moi/claude-2', name: 'claude-2', kind: 'CLAUDE', status: 'IDLE', projects: [], current_job: null, auth_status: 'NEEDS_AUTH' },
    ];
    routes['GET /api/my/machines'] = () => ok([machine(agents)]);
    routes['DELETE /api/my/machines/mac-moi/agents/claude-2'] = () => { agents = agents.slice(0, 1); return ok({ removed: 'mac-moi/claude-2' }); };
    render(wrap(<MyMachines />, '/my/machines'));
    const row = await screen.findByTestId('my-agent-mac-moi/claude-2');
    expect(row.textContent).toMatch(/agenthub account login claude-2/);
    fireEvent.click(within(row).getByRole('button', { name: 'Xoá' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url === '/api/my/machines/mac-moi/agents/claude-2')).toBe(true));
    await waitFor(() => expect(screen.queryByTestId('my-agent-mac-moi/claude-2')).toBeNull());
  });
});
