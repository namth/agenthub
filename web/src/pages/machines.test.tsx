// Test nghiệm thu TASK-024 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/join-machine.md §2, §4, §6; docs/features/multi-project.md §4 (SCR-05 chọn dự án cho agent).
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import AddMachineDialog from '../components/AddMachineDialog';
import MyMachines from './MyMachines';
import Fleet from './Fleet';

type Call = { url: string; method: string; body?: unknown };
let calls: Call[] = [];
let routes: Record<string, () => { status: number; body: unknown }> = {};
let clip: string[] = [];

function ok(data: unknown, status = 200) {
  return { status, body: { success: true, data, error: null } };
}
function fail(status: number, code: string, message = code) {
  return { status, body: { success: false, data: null, error: { code, message } } };
}

beforeEach(() => {
  calls = [];
  routes = {};
  clip = [];
  vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ url: input, method, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const key = `${method} ${input.split('?')[0]}`;
    const h = routes[key];
    if (!h) return new Response(JSON.stringify(fail(404, 'NOT_FOUND').body), { status: 404 });
    const r = h();
    if (r.status === 204) return new Response(null, { status: 204 });
    return new Response(JSON.stringify(r.body), { status: r.status, headers: { 'Content-Type': 'application/json' } });
  });
  vi.stubGlobal('confirm', () => true);
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: async (s: string) => { clip.push(s); } },
  });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const wrap = (ui: React.ReactNode, path = '/') => (
  <MemoryRouter initialEntries={[path]}>
    <Routes>
      <Route path="/" element={ui} />
      <Route path="/fleet" element={ui} />
      <Route path="/my/machines" element={ui} />
    </Routes>
  </MemoryRouter>
);

const inMinutes = (m: number) => new Date(Date.now() + m * 60_000).toISOString();
const joinResp = (expires = inMinutes(15)) => ({
  code: 'ABCD-EFGH',
  expires_at: expires,
  command: 'curl -fsSL https://agent.oa.io.vn/install.sh | sh -s -- --join ABCD-EFGH --name mac-moi',
  agent_prompt: '# Nhiệm vụ: nối máy này vào AgentHub (https://agent.oa.io.vn) với tên "mac-moi"\nMã tham gia: ABCD-EFGH',
});
const machine = (over: Record<string, unknown> = {}) => ({
  id: 'mac-moi', name: 'mac-moi', os: 'darwin', conn_status: 'ONLINE', version: 'v0.3.0', paused: false,
  agents: [{ id: 'mac-moi/claude', kind: 'CLAUDE', status: 'IDLE', projects: [], current_job: null }],
  ...over,
});
const projects = [
  { id: 'p1', name: 'agenthub', github_repo: 'namth/agenthub', status: 'ACTIVE', role: 'ADMIN' },
  { id: 'p2', name: 'edusystem', github_repo: 'namth/edusystem', status: 'ACTIVE', role: 'PARTNER' },
];

// ---------- Hộp thoại Thêm máy ----------

describe('Hộp thoại Thêm máy', () => {
  const create = (name = 'mac-moi') => {
    fireEvent.change(screen.getByLabelText('Tên máy'), { target: { value: name } });
    fireEvent.click(screen.getByRole('button', { name: 'Tạo mã' }));
  };

  it('đóng → không hiện gì', () => {
    render(wrap(<AddMachineDialog open={false} onClose={() => {}} />));
    expect(screen.queryByLabelText('Tên máy')).toBeNull();
  });

  it('validate tên máy phía client', async () => {
    render(wrap(<AddMachineDialog open onClose={() => {}} />));
    for (const bad of ['a', 'có-dấu', 'có khoảng trắng', 'x'.repeat(41)]) {
      create(bad);
      expect(await screen.findByText(/2–40 ký tự/)).toBeTruthy();
    }
    expect(calls.some((c) => c.method === 'POST')).toBe(false);
  });

  it('tạo mã → hiện lệnh + hướng dẫn agent, sao chép được, đếm ngược', async () => {
    routes['POST /api/fleet/join-codes'] = () => ok(joinResp(), 201);
    routes['GET /api/my/machines'] = () => ok([]);
    render(wrap(<AddMachineDialog open onClose={() => {}} />));
    create();
    const cmd = await screen.findByTestId('join-command');
    expect(calls.find((c) => c.method === 'POST')!.body).toEqual({ name: 'mac-moi' });
    expect(cmd.textContent).toContain('--join ABCD-EFGH');
    expect(screen.getByTestId('join-prompt').textContent).toContain('# Nhiệm vụ: nối máy này vào AgentHub');
    fireEvent.click(screen.getByRole('button', { name: 'Sao chép lệnh' }));
    fireEvent.click(screen.getByRole('button', { name: 'Sao chép hướng dẫn' }));
    await waitFor(() => expect(clip).toEqual([joinResp().command, joinResp().agent_prompt]));
    expect(screen.getByText(/Còn 1[45]:\d{2}/)).toBeTruthy();
    expect(screen.getByText(/Đang chờ máy kết nối/)).toBeTruthy();
  });

  it('máy cùng tên ONLINE xuất hiện trong /api/my/machines → "Đã kết nối"', async () => {
    routes['POST /api/fleet/join-codes'] = () => ok(joinResp(), 201);
    routes['GET /api/my/machines'] = () => ok([machine()]);
    render(wrap(<AddMachineDialog open onClose={() => {}} />));
    create();
    expect(await screen.findByText(/Đã kết nối/)).toBeTruthy();
  });

  it('thăm dò /api/my/machines mỗi 3 giây cho tới khi máy kết nối', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    let online = false;
    routes['POST /api/fleet/join-codes'] = () => ok(joinResp(), 201);
    routes['GET /api/my/machines'] = () => ok(online ? [machine()] : [machine({ name: 'may-khac', id: 'may-khac' })]);
    render(wrap(<AddMachineDialog open onClose={() => {}} />));
    create();
    await screen.findByTestId('join-command');
    await waitFor(() => expect(calls.filter((c) => c.url === '/api/my/machines').length).toBeGreaterThanOrEqual(1));
    const before = calls.filter((c) => c.url === '/api/my/machines').length;
    online = true;
    await act(async () => { await vi.advanceTimersByTimeAsync(3100); });
    await waitFor(() => expect(calls.filter((c) => c.url === '/api/my/machines').length).toBeGreaterThan(before));
    expect(await screen.findByText(/Đã kết nối/)).toBeTruthy();
    const after = calls.filter((c) => c.url === '/api/my/machines').length;
    await act(async () => { await vi.advanceTimersByTimeAsync(6500); });
    expect(calls.filter((c) => c.url === '/api/my/machines').length).toBe(after); // đã kết nối thì ngừng thăm dò
  });

  it('mã hết hạn → báo hết hạn và nút Tạo mã mới', async () => {
    routes['POST /api/fleet/join-codes'] = () => ok(joinResp(inMinutes(-1)), 201);
    routes['GET /api/my/machines'] = () => ok([]);
    render(wrap(<AddMachineDialog open onClose={() => {}} />));
    create();
    expect(await screen.findByText(/Mã đã hết hạn/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Tạo mã mới' }));
    await waitFor(() => expect(calls.filter((c) => c.method === 'POST').length).toBe(2));
  });

  it('tên trùng máy người khác (409) → banner lỗi', async () => {
    routes['POST /api/fleet/join-codes'] = () => fail(409, 'CONFLICT', 'tên máy đã được dùng');
    render(wrap(<AddMachineDialog open onClose={() => {}} />));
    create();
    expect((await screen.findByRole('alert')).textContent).toMatch(/đã được dùng/);
  });

  it('nút Đóng gọi onClose', () => {
    const onClose = vi.fn();
    render(wrap(<AddMachineDialog open onClose={onClose} />));
    fireEvent.click(screen.getByRole('button', { name: 'Đóng' }));
    expect(onClose).toHaveBeenCalled();
  });
});

// ---------- SCR-11 Máy của tôi ----------

describe('SCR-11 Máy của tôi', () => {
  beforeEach(() => {
    routes['GET /api/projects'] = () => ok(projects);
  });

  it('đang tải → skeleton; rỗng → "Chưa có máy nào" + Thêm máy mới mở hộp thoại', async () => {
    routes['GET /api/my/machines'] = () => ok([]);
    const { container } = render(wrap(<MyMachines />, '/my/machines'));
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(await screen.findByText('Chưa có máy nào')).toBeTruthy();
    fireEvent.click(screen.getAllByRole('button', { name: /Thêm máy mới/ })[0]);
    expect(await screen.findByLabelText('Tên máy')).toBeTruthy();
  });

  it('danh sách máy: trạng thái, phiên bản, agent, việc đang chạy', async () => {
    routes['GET /api/my/machines'] = () => ok([machine({
      agents: [{ id: 'mac-moi/claude', kind: 'CLAUDE', status: 'BUSY', projects: [], current_job: { job_id: 'j1', task_id: 't77', kind: 'CODE', attempt: 1 } }],
    })]);
    render(wrap(<MyMachines />, '/my/machines'));
    const m = await screen.findByTestId('machine-mac-moi');
    expect(within(m).getByText('mac-moi')).toBeTruthy();
    expect(within(m).getByText(/ONLINE/)).toBeTruthy();
    expect(within(m).getByText(/v0\.3\.0/)).toBeTruthy();
    expect(within(m).getByText(/CLAUDE/)).toBeTruthy();
    expect(within(m).getByText(/t77/)).toBeTruthy();
  });

  it('Tạm dừng / Tiếp tục → PATCH paused', async () => {
    routes['GET /api/my/machines'] = () => ok([machine(), machine({ id: 'may-2', name: 'may-2', paused: true })]);
    routes['PATCH /api/my/machines/mac-moi'] = () => ok({});
    routes['PATCH /api/my/machines/may-2'] = () => ok({});
    render(wrap(<MyMachines />, '/my/machines'));
    fireEvent.click(within(await screen.findByTestId('machine-mac-moi')).getByRole('button', { name: 'Tạm dừng' }));
    fireEvent.click(within(screen.getByTestId('machine-may-2')).getByRole('button', { name: 'Tiếp tục' }));
    await waitFor(() => expect(calls.filter((c) => c.method === 'PATCH').length).toBe(2));
    expect(calls.find((c) => c.url === '/api/my/machines/mac-moi' && c.method === 'PATCH')!.body).toEqual({ paused: true });
    expect(calls.find((c) => c.url === '/api/my/machines/may-2' && c.method === 'PATCH')!.body).toEqual({ paused: false });
  });

  it('Gỡ (xác nhận) → DELETE 204 rồi tải lại; huỷ thì không gọi', async () => {
    routes['GET /api/my/machines'] = () => ok([machine()]);
    routes['DELETE /api/my/machines/mac-moi'] = () => ({ status: 204, body: null });
    render(wrap(<MyMachines />, '/my/machines'));
    const m = await screen.findByTestId('machine-mac-moi');
    vi.stubGlobal('confirm', () => false);
    fireEvent.click(within(m).getByRole('button', { name: 'Gỡ' }));
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false);
    vi.stubGlobal('confirm', () => true);
    fireEvent.click(within(m).getByRole('button', { name: 'Gỡ' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url === '/api/my/machines/mac-moi')).toBe(true));
    await waitFor(() => expect(calls.filter((c) => c.method === 'GET' && c.url === '/api/my/machines').length).toBe(2));
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('chọn dự án nhận việc → PATCH projects; "Mọi dự án" → null', async () => {
    routes['GET /api/my/machines'] = () => ok([machine()]);
    routes['PATCH /api/my/machines/mac-moi'] = () => ok({});
    render(wrap(<MyMachines />, '/my/machines'));
    const m = await screen.findByTestId('machine-mac-moi');
    const sel = within(m).getByLabelText('Dự án nhận việc') as HTMLSelectElement;
    await waitFor(() => expect(within(sel).getByText('namth/edusystem')).toBeTruthy());
    expect(sel.value).toBe('');
    fireEvent.change(sel, { target: { value: 'p2' } });
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ projects: ['p2'] }));
    fireEvent.change(sel, { target: { value: '' } });
    await waitFor(() => expect(calls.filter((c) => c.method === 'PATCH')[1]?.body).toEqual({ projects: null }));
  });

  it('PATCH bị 403 → báo lỗi', async () => {
    routes['GET /api/my/machines'] = () => ok([machine()]);
    routes['PATCH /api/my/machines/mac-moi'] = () => fail(403, 'FORBIDDEN', 'không là thành viên dự án');
    render(wrap(<MyMachines />, '/my/machines'));
    const m = await screen.findByTestId('machine-mac-moi');
    const sel = within(m).getByLabelText('Dự án nhận việc');
    await waitFor(() => expect(within(sel).getByText('namth/edusystem')).toBeTruthy());
    fireEvent.change(sel, { target: { value: 'p2' } });
    expect((await screen.findByRole('alert')).textContent).toMatch(/thành viên/);
  });

  it('lỗi tải → banner Thử lại', async () => {
    let n = 0;
    routes['GET /api/my/machines'] = () => (++n === 1 ? fail(500, 'INTERNAL') : ok([machine()]));
    render(wrap(<MyMachines />, '/my/machines'));
    fireEvent.click(within(await screen.findByRole('alert')).getByRole('button', { name: 'Thử lại' }));
    await screen.findByTestId('machine-mac-moi');
  });
});

// ---------- SCR-05 Cụm máy nhóm theo chủ ----------

const lead = { state: 'HELD', agent_id: 'sys/claude', epoch: 3, expires_at: null, vacant_since: null, eta: null, waiting_jobs: 0 };
const agent = (id: string, kind: string, over: Record<string, unknown> = {}) => ({
  id, kind, status: 'IDLE', cooldown_until: null, lead_priority: null, soft_max_jobs: null, soft_window_hours: 5, jobs_in_window: 0,
  current_job: null, lead_eligible: true, projects: null, ...over,
});
const fleet = {
  lead,
  workers: [
    { id: 'sys', name: 'sys', os: 'linux', conn_status: 'ONLINE', offline_since: null, owner: null, version: 'v0.3.0', paused: false,
      agents: [agent('sys/claude', 'CLAUDE')] },
    { id: 'mb', name: 'mb', os: 'darwin', conn_status: 'ONLINE', offline_since: null, owner: { id: 'u2', login: 'doi-tac' }, version: 'v0.2.9', paused: true,
      agents: [
        agent('mb/claude', 'CLAUDE', { lead_eligible: false }),
        agent('mb/antigravity', 'ANTIGRAVITY', { status: 'BUSY', projects: ['p2'],
          current_job: { job_id: 'j9', task_id: 't9', kind: 'CODE', attempt: 2, project_id: 'p2', project: 'namth/edusystem', task_title: 'Trang điểm danh' } }),
      ] },
  ],
};

describe('SCR-05 Cụm máy nhóm theo chủ', () => {
  beforeEach(() => {
    routes['GET /api/fleet'] = () => ok(fleet);
    routes['GET /api/projects'] = () => ok(projects);
  });

  it('nhóm theo chủ máy: Hệ thống và từng partner', async () => {
    render(wrap(<Fleet />, '/fleet'));
    const sys = await screen.findByTestId('owner-group-system');
    expect(within(sys).getByText('Hệ thống')).toBeTruthy();
    expect(within(sys).getByTestId('worker-sys')).toBeTruthy();
    const u2 = screen.getByTestId('owner-group-u2');
    expect(within(u2).getByText('doi-tac')).toBeTruthy();
    expect(within(u2).getByTestId('worker-mb')).toBeTruthy();
  });

  it('phiên bản, máy tạm dừng, dự án + task đang chạy', async () => {
    render(wrap(<Fleet />, '/fleet'));
    const mb = await screen.findByTestId('worker-mb');
    expect(within(mb).getByText(/v0\.2\.9/)).toBeTruthy();
    expect(within(mb).getByText(/Tạm dừng/)).toBeTruthy();
    const ag = within(mb).getByTestId('agent-mb/antigravity');
    expect(within(ag).getByText(/namth\/edusystem/)).toBeTruthy();
    expect(within(ag).getByText(/Trang điểm danh/)).toBeTruthy();
  });

  it('công tắc "Cho làm trưởng phòng" chỉ ở agent Claude, bật/tắt → PATCH lead_eligible', async () => {
    routes['PATCH /api/admin/agents/mb%2Fclaude'] = () => ok({ id: 'mb/claude' });
    render(wrap(<Fleet />, '/fleet'));
    const mbc = await screen.findByTestId('agent-mb/claude');
    const sw = within(mbc).getByLabelText('Cho làm trưởng phòng') as HTMLInputElement;
    expect(sw.checked).toBe(false);
    expect((within(screen.getByTestId('agent-sys/claude')).getByLabelText('Cho làm trưởng phòng') as HTMLInputElement).checked).toBe(true);
    expect(within(screen.getByTestId('agent-mb/antigravity')).queryByLabelText('Cho làm trưởng phòng')).toBeNull();
    fireEvent.click(sw);
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true));
    const p = calls.find((c) => c.method === 'PATCH')!;
    expect(p.url).toBe('/api/admin/agents/mb%2Fclaude');
    expect(p.body).toEqual({ lead_eligible: true });
  });

  it('ô chọn dự án được nhận của agent → PATCH projects; "Mọi dự án" → null', async () => {
    routes['PATCH /api/admin/agents/mb%2Fantigravity'] = () => ok({ id: 'mb/antigravity' });
    render(wrap(<Fleet />, '/fleet'));
    const ag = await screen.findByTestId('agent-mb/antigravity');
    const sel = within(ag).getByLabelText('Dự án được nhận') as HTMLSelectElement;
    await waitFor(() => expect(sel.value).toBe('p2'));
    fireEvent.change(sel, { target: { value: '' } });
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ projects: null }));
    fireEvent.change(sel, { target: { value: 'p1' } });
    await waitFor(() => expect(calls.filter((c) => c.method === 'PATCH')[1]?.body).toEqual({ projects: ['p1'] }));
    expect((within(screen.getByTestId('agent-sys/claude')).getByLabelText('Dự án được nhận') as HTMLSelectElement).value).toBe('');
  });

  it('nút Thêm máy mới mở hộp thoại', async () => {
    render(wrap(<Fleet />, '/fleet'));
    await screen.findByTestId('owner-group-system');
    fireEvent.click(screen.getByRole('button', { name: /Thêm máy mới/ }));
    expect(await screen.findByLabelText('Tên máy')).toBeTruthy();
  });
});
