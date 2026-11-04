// Test nghiệm thu TASK-008 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/03 §1, §3, §4; docs/features/lead-rotation.md §4; hợp đồng API khoá ở
// internal/api/fleet_test.go và internal/tasks/tasks_test.go.
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import Fleet from './Fleet';
import Kanban from './Kanban';
import TaskCreate from './TaskCreate';

type Call = { url: string; method: string; body?: unknown };
let calls: Call[] = [];
let routes: Record<string, () => { status: number; body: unknown }> = {};

function ok(data: unknown, status = 200) {
  return { status, body: { success: true, data, error: null } };
}
function fail(status: number, code: string) {
  return { status, body: { success: false, data: null, error: { code, message: code } } };
}

beforeEach(() => {
  calls = [];
  routes = {};
  vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ url: input, method, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const key = `${method} ${input.split('?')[0]}`;
    const h = routes[key];
    if (!h) return new Response(JSON.stringify(fail(404, 'NOT_FOUND').body), { status: 404 });
    const r = h();
    return new Response(JSON.stringify(r.body), { status: r.status, headers: { 'Content-Type': 'application/json' } });
  });
  vi.stubGlobal('confirm', () => true);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const wrap = (ui: React.ReactNode, path = '/') => (
  <MemoryRouter initialEntries={[path]}>
    <Routes>
      <Route path="/" element={ui} />
      <Route path="/fleet" element={ui} />
      <Route path="/tasks/new" element={ui} />
      <Route path="/tasks/:id" element={<div>Trang chi tiết task</div>} />
    </Routes>
  </MemoryRouter>
);

// ---------- SCR-05 Fleet ----------

const fleetHeld = {
  lead: { state: 'HELD', agent_id: 'mac-mini/claude', epoch: 7, expires_at: '2026-11-04T10:01:30Z', vacant_since: null, eta: null, waiting_jobs: 0 },
  workers: [
    { id: 'laptop', name: 'laptop', os: 'darwin', conn_status: 'ONLINE', offline_since: null, agents: [
      { id: 'laptop/claude', kind: 'CLAUDE', status: 'IDLE', cooldown_until: null, lead_priority: 2, soft_max_jobs: 20, soft_window_hours: 5, jobs_in_window: 3, current_job: null },
    ] },
    { id: 'mac-mini', name: 'mac-mini', os: 'darwin', conn_status: 'ONLINE', offline_since: null, agents: [
      { id: 'mac-mini/claude', kind: 'CLAUDE', status: 'BUSY', cooldown_until: null, lead_priority: 1, soft_max_jobs: null, soft_window_hours: 5, jobs_in_window: 1, current_job: { job_id: 'j1', task_id: 't101', kind: 'SPEC', attempt: 1 } },
      { id: 'mac-mini/antigravity', kind: 'ANTIGRAVITY', status: 'COOLDOWN', cooldown_until: '2026-11-04T11:40:00Z', lead_priority: null, soft_max_jobs: null, soft_window_hours: 5, jobs_in_window: 0, current_job: null },
    ] },
    { id: 'server', name: 'server', os: 'linux', conn_status: 'OFFLINE', offline_since: '2026-11-04T09:58:00Z', agents: [
      { id: 'server/claude', kind: 'CLAUDE', status: 'IDLE', cooldown_until: null, lead_priority: null, soft_max_jobs: null, soft_window_hours: 5, jobs_in_window: 0, current_job: null },
    ] },
  ],
};

describe('SCR-05 Fleet', () => {
  it('đang tải → skeleton (aria-busy)', async () => {
    routes['GET /api/fleet'] = () => ok(fleetHeld);
    const { container } = render(wrap(<Fleet />, '/fleet'));
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    await screen.findByText(/epoch 7/);
  });

  it('rỗng → thông điệp và nút thêm máy', async () => {
    routes['GET /api/fleet'] = () => ok({ lead: { ...fleetHeld.lead, state: 'VACANT', agent_id: null }, workers: [] });
    render(wrap(<Fleet />, '/fleet'));
    expect(await screen.findByText('Chưa có máy nào kết nối')).toBeTruthy();
    expect(screen.getByRole('button', { name: /Thêm máy mới/ })).toBeTruthy();
  });

  it('lỗi → banner role=alert, Thử lại gọi lại API', async () => {
    let n = 0;
    routes['GET /api/fleet'] = () => (++n === 1 ? fail(500, 'INTERNAL') : ok(fleetHeld));
    render(wrap(<Fleet />, '/fleet'));
    const alert = await screen.findByRole('alert');
    fireEvent.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await screen.findByText(/epoch 7/);
    expect(calls.filter((c) => c.url.startsWith('/api/fleet')).length).toBe(2);
  });

  it('hiển thị lead, agent, trạng thái và việc đang làm', async () => {
    routes['GET /api/fleet'] = () => ok(fleetHeld);
    render(wrap(<Fleet />, '/fleet'));
    const card = await screen.findByTestId('lead-card');
    expect(within(card).getByText(/mac-mini\/claude/)).toBeTruthy();
    expect(within(card).getByText(/epoch 7/)).toBeTruthy();
    const busy = screen.getByTestId('agent-mac-mini/claude');
    expect(within(busy).getByText(/BUSY/)).toBeTruthy();
    expect(within(busy).getByText(/t101/)).toBeTruthy();
    expect(within(screen.getByTestId('agent-mac-mini/antigravity')).getByText(/COOLDOWN/)).toBeTruthy();
    expect(within(screen.getByTestId('worker-server')).getByText(/OFFLINE/)).toBeTruthy();
  });

  it('nút Trao vai chỉ ở agent Claude IDLE trên máy ONLINE, không phải lead', async () => {
    routes['GET /api/fleet'] = () => ok(fleetHeld);
    render(wrap(<Fleet />, '/fleet'));
    await screen.findByTestId('lead-card');
    const buttons = screen.getAllByRole('button', { name: 'Trao vai' });
    expect(buttons.length).toBe(1);
    expect(within(screen.getByTestId('agent-laptop/claude')).getByRole('button', { name: 'Trao vai' })).toBeTruthy();
  });

  it('Trao vai → POST assign rồi tải lại', async () => {
    routes['GET /api/fleet'] = () => ok(fleetHeld);
    routes['POST /api/admin/lead/assign'] = () => ok({ state: 'HELD', agent_id: 'laptop/claude', epoch: 8, expires_at: null });
    render(wrap(<Fleet />, '/fleet'));
    await screen.findByTestId('lead-card');
    fireEvent.click(within(screen.getByTestId('agent-laptop/claude')).getByRole('button', { name: 'Trao vai' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/admin/lead/assign')).toBe(true));
    const post = calls.find((c) => c.url === '/api/admin/lead/assign')!;
    expect(post.body).toEqual({ agent_id: 'laptop/claude' });
    await waitFor(() => expect(calls.filter((c) => c.url.startsWith('/api/fleet')).length).toBe(2));
  });

  it('Trao vai bị 409 → thông báo không đủ điều kiện', async () => {
    routes['GET /api/fleet'] = () => ok(fleetHeld);
    routes['POST /api/admin/lead/assign'] = () => fail(409, 'AGENT_NOT_ELIGIBLE');
    render(wrap(<Fleet />, '/fleet'));
    await screen.findByTestId('lead-card');
    fireEvent.click(screen.getByRole('button', { name: 'Trao vai' }));
    expect(await screen.findByText(/không còn đủ điều kiện nhận vai/)).toBeTruthy();
  });

  it('Thu hồi vai → POST revoke; huỷ xác nhận thì không gọi', async () => {
    routes['GET /api/fleet'] = () => ok(fleetHeld);
    routes['POST /api/admin/lead/revoke'] = () => ok({ state: 'VACANT', agent_id: null, epoch: 8, expires_at: null });
    render(wrap(<Fleet />, '/fleet'));
    const card = await screen.findByTestId('lead-card');
    vi.stubGlobal('confirm', () => false);
    fireEvent.click(within(card).getByRole('button', { name: 'Thu hồi vai' }));
    expect(calls.some((c) => c.url === '/api/admin/lead/revoke')).toBe(false);
    vi.stubGlobal('confirm', () => true);
    fireEvent.click(within(card).getByRole('button', { name: 'Thu hồi vai' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/admin/lead/revoke')).toBe(true));
  });

  it('trống lead → thẻ cảnh báo, giờ dự kiến và số việc chờ', async () => {
    routes['GET /api/fleet'] = () => ok({ ...fleetHeld, lead: { state: 'VACANT', agent_id: null, epoch: 8, expires_at: null, vacant_since: '2026-11-04T10:00:00Z', eta: '2026-11-04T11:40:00Z', waiting_jobs: 3 } });
    render(wrap(<Fleet />, '/fleet'));
    const card = await screen.findByTestId('lead-card');
    expect(within(card).getByText(/Chưa có trưởng phòng/)).toBeTruthy();
    expect(within(card).getByText(/3/)).toBeTruthy();
    expect(within(card).queryByRole('button', { name: 'Thu hồi vai' })).toBeNull();
  });
});

// ---------- SCR-02 Kanban ----------

const board = {
  columns: [
    { key: 'backlog', title: 'Chờ duyệt', tasks: [{ id: 't1', title: 'Xuất PDF', status: 'PENDING_APPROVAL', github_issue_id: 101, parent_id: null, created_at: '2026-11-04T09:00:00Z' }] },
    { key: 'planning', title: 'Lập kế hoạch', tasks: [] },
    { key: 'in_progress', title: 'Đang làm', tasks: [{ id: 't2', title: 'Schema DB', status: 'IN_PROGRESS', github_issue_id: 102, parent_id: null, created_at: '2026-11-04T09:00:00Z' }] },
    { key: 'review', title: 'Kiểm định', tasks: [] },
    { key: 'done', title: 'Xong', tasks: [] },
  ],
};

describe('SCR-02 Kanban', () => {
  it('hiển thị đủ 5 cột và thẻ', async () => {
    routes['GET /api/tasks'] = () => ok(board);
    render(wrap(<Kanban role="ADMIN" />));
    for (const c of board.columns) expect(await screen.findByTestId(`column-${c.key}`)).toBeTruthy();
    expect(within(screen.getByTestId('column-backlog')).getByText('Xuất PDF')).toBeTruthy();
    expect(within(screen.getByTestId('column-in_progress')).getByText('Schema DB')).toBeTruthy();
  });

  it('rỗng → thông điệp và nút tạo yêu cầu', async () => {
    routes['GET /api/tasks'] = () => ok({ columns: board.columns.map((c) => ({ ...c, tasks: [] })) });
    render(wrap(<Kanban role="PARTNER" />));
    expect(await screen.findByText('Chưa có tác vụ nào trong hàng đợi')).toBeTruthy();
    expect(screen.getByRole('link', { name: /Tạo yêu cầu đầu tiên/ }).getAttribute('href')).toBe('/tasks/new');
  });

  it('Admin duyệt thẻ chờ duyệt → POST approve rồi tải lại', async () => {
    routes['GET /api/tasks'] = () => ok(board);
    routes['POST /api/tasks/t1/approve'] = () => ok({ id: 't1', status: 'PLANNING' });
    render(wrap(<Kanban role="ADMIN" />));
    const col = await screen.findByTestId('column-backlog');
    fireEvent.click(within(col).getByRole('button', { name: 'Duyệt' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/tasks/t1/approve')).toBe(true));
    await waitFor(() => expect(calls.filter((c) => c.url.startsWith('/api/tasks') && c.method === 'GET').length).toBe(2));
  });

  it('Partner không có nút Duyệt', async () => {
    routes['GET /api/tasks'] = () => ok(board);
    render(wrap(<Kanban role="PARTNER" />));
    await screen.findByTestId('column-backlog');
    expect(screen.queryByRole('button', { name: 'Duyệt' })).toBeNull();
  });

  it('lỗi tải → banner Thử lại', async () => {
    routes['GET /api/tasks'] = () => fail(500, 'INTERNAL');
    render(wrap(<Kanban role="ADMIN" />));
    expect(within(await screen.findByRole('alert')).getByRole('button', { name: 'Thử lại' })).toBeTruthy();
  });
});

// ---------- SCR-03 Tạo yêu cầu ----------

describe('SCR-03 Tạo yêu cầu', () => {
  const fill = (title: string, desc: string) => {
    fireEvent.change(screen.getByLabelText('Tiêu đề'), { target: { value: title } });
    fireEvent.change(screen.getByLabelText('Mô tả chi tiết'), { target: { value: desc } });
    fireEvent.click(screen.getByRole('button', { name: 'Gửi yêu cầu' }));
  };
  const desc = 'Khách hàng cần xuất bảng tiến độ thành PDF.';

  it('validate phía client, không gọi API khi sai', async () => {
    render(wrap(<TaskCreate />, '/tasks/new'));
    fill('ngắn', 'quá ngắn');
    expect(await screen.findByText(/tối thiểu 6 ký tự/)).toBeTruthy();
    expect(screen.getByText(/tối thiểu 20 ký tự/)).toBeTruthy();
    fill('a'.repeat(121), desc);
    expect(await screen.findByText(/tối đa 120 ký tự/)).toBeTruthy();
    expect(calls.length).toBe(0);
  });

  it('gửi hợp lệ → POST /api/tasks và chuyển sang trang chi tiết', async () => {
    routes['POST /api/tasks'] = () => ok({ id: 'tnew', status: 'PENDING_APPROVAL' }, 201);
    render(wrap(<TaskCreate />, '/tasks/new'));
    fill('  Xuất báo cáo PDF  ', desc);
    expect(await screen.findByText('Trang chi tiết task')).toBeTruthy();
    expect(calls[0]).toEqual({ url: '/api/tasks', method: 'POST', body: { title: 'Xuất báo cáo PDF', description: desc } });
  });

  it('server lỗi → giữ dữ liệu form và báo lỗi', async () => {
    routes['POST /api/tasks'] = () => fail(502, 'GITHUB_ERROR');
    render(wrap(<TaskCreate />, '/tasks/new'));
    fill('Xuất báo cáo PDF', desc);
    expect(await screen.findByRole('alert')).toBeTruthy();
    expect((screen.getByLabelText('Tiêu đề') as HTMLInputElement).value).toBe('Xuất báo cáo PDF');
  });
});
