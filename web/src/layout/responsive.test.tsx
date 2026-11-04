// Test nghiệm thu TASK-047 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/remote-command.md §4 "Web trên điện thoại", BR-65; TC-96.
// jsdom không đo bố cục: test kiểm hành vi menu + quy ước lớp responsive; hiển thị thật kiểm bằng trình duyệt khi nghiệm thu.
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import AppShell from './AppShell';
import Kanban from '../pages/Kanban';
import TaskDetail from '../pages/TaskDetail';
import MyMachines from '../pages/MyMachines';
import Fleet from '../pages/Fleet';

let routes: Record<string, () => { status: number; body: unknown }> = {};
const ok = (data: unknown) => ({ status: 200, body: { success: true, data, error: null } });

beforeEach(() => {
  routes = {};
  vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase();
    const h = routes[`${method} ${input.split('?')[0]}`];
    const r = h ? h() : { status: 404, body: { success: false, data: null, error: { code: 'NOT_FOUND', message: '' } } };
    return new Response(JSON.stringify(r.body), { status: r.status, headers: { 'Content-Type': 'application/json' } });
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const user = { id: 'u1', github_id: '1', username: 'namth', avatar_url: '', role: 'ADMIN' as const };
const shell = () =>
  render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route element={<AppShell user={user} officeEnabled />}>
          <Route path="/" element={<div>Trang chủ</div>} />
          <Route path="/fleet" element={<div>Trang cụm máy</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
const cls = (el: Element | null) => (el?.getAttribute('class') ?? '').split(/\s+/);

describe('AppShell trên điện thoại', () => {
  beforeEach(() => {
    routes['GET /api/projects'] = () => ok([]);
  });

  it('sidebar ẩn dưới md, có thanh trên cùng với nút Mở menu', () => {
    const { container } = shell();
    const aside = screen.getByTestId('sidebar');
    expect(cls(aside)).toEqual(expect.arrayContaining(['hidden', 'md:flex']));
    const header = screen.getByTestId('mobile-header');
    expect(cls(header)).toContain('md:hidden');
    const btn = within(header).getByRole('button', { name: 'Mở menu' });
    expect(btn.getAttribute('aria-expanded')).toBe('false');
    const main = container.querySelector('main');
    expect(cls(main)).toEqual(expect.arrayContaining(['min-w-0', 'p-4', 'md:p-6']));
  });

  it('mở menu → ngăn kéo có đủ mục; bấm mục thì chuyển trang và đóng', async () => {
    shell();
    fireEvent.click(screen.getByRole('button', { name: 'Mở menu' }));
    const drawer = screen.getByRole('dialog', { name: 'Menu' });
    expect(screen.getByRole('button', { name: 'Mở menu' }).getAttribute('aria-expanded')).toBe('true');
    for (const label of ['Kanban', 'Cụm máy', 'Văn phòng', 'Máy của tôi']) {
      expect(within(drawer).getByRole('link', { name: label })).toBeTruthy();
    }
    expect(drawer.textContent).toContain('namth');
    fireEvent.click(within(drawer).getByRole('link', { name: 'Cụm máy' }));
    expect(await screen.findByText('Trang cụm máy')).toBeTruthy();
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Menu' })).toBeNull());
  });

  it('đóng bằng nền mờ và phím Escape', async () => {
    shell();
    fireEvent.click(screen.getByRole('button', { name: 'Mở menu' }));
    fireEvent.click(screen.getByTestId('menu-backdrop'));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Menu' })).toBeNull());
    fireEvent.click(screen.getByRole('button', { name: 'Mở menu' }));
    act(() => { fireEvent.keyDown(document, { key: 'Escape' }); });
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Menu' })).toBeNull());
  });
});

describe('Trang chính dùng được ở 375px', () => {
  it('Kanban cuộn ngang theo cột', async () => {
    routes['GET /api/tasks'] = () => ok({ columns: ['backlog', 'planning', 'in_progress', 'review', 'done'].map((key) => ({ key, title: key, tasks: key === 'planning' ? [{ id: 't1', title: 'Việc A', status: 'PLANNING', github_issue_id: 1, parent_id: null, created_at: '2026-11-04T09:00:00Z' }] : [] })) });
    routes['GET /api/projects'] = () => ok([]);
    render(<MemoryRouter><Kanban role="ADMIN" /></MemoryRouter>);
    const board = await screen.findByTestId('kanban-board');
    expect(cls(board)).toEqual(expect.arrayContaining(['overflow-x-auto', 'snap-x']));
    const cols = Array.from(board.children);
    expect(cols.length).toBeGreaterThanOrEqual(5);
    for (const c of cols) expect(cls(c)).toContain('shrink-0');
  });

  it('SCR-04: nút Duyệt / Từ chối / Merge đủ lớn để bấm bằng ngón tay', async () => {
    routes['GET /api/tasks/t1'] = () => ok({ id: 't1', title: 'Xuất PDF', description: 'mô tả đủ dài để hợp lệ', status: 'PENDING_APPROVAL', github_issue_id: 1, pr_url: null, parent_id: null, created_at: '2026-11-04T09:00:00Z', subtasks: [] });
    render(<MemoryRouter initialEntries={['/tasks/t1']}><Routes><Route path="/tasks/:id" element={<TaskDetail role="ADMIN" />} /></Routes></MemoryRouter>);
    for (const name of ['Duyệt', 'Từ chối']) {
      expect(cls(await screen.findByRole('button', { name }))).toContain('min-h-11');
    }
    cleanup();
    routes['GET /api/tasks/t1'] = () => ok({ id: 't1', title: 'Xuất PDF', description: 'mô tả đủ dài để hợp lệ', status: 'QA_PASSED', github_issue_id: 1, pr_url: 'https://github.com/x/y/pull/1', parent_id: null, created_at: '2026-11-04T09:00:00Z', subtasks: [] });
    render(<MemoryRouter initialEntries={['/tasks/t1']}><Routes><Route path="/tasks/:id" element={<TaskDetail role="ADMIN" />} /></Routes></MemoryRouter>);
    expect(cls(await screen.findByRole('button', { name: /Merge/ }))).toContain('min-h-11');
  });

  it('SCR-11: nút Tạm dừng, Gỡ, Thêm máy mới đủ lớn', async () => {
    routes['GET /api/projects'] = () => ok([]);
    routes['GET /api/my/machines'] = () => ok([{ id: 'm1', name: 'm1', os: 'darwin', conn_status: 'ONLINE', version: 'v1', paused: false, agents: [] }]);
    render(<MemoryRouter initialEntries={['/my/machines']}><MyMachines /></MemoryRouter>);
    const m = await screen.findByTestId('machine-m1');
    expect(cls(within(m).getByRole('button', { name: 'Tạm dừng' }))).toContain('min-h-11');
    expect(cls(within(m).getByRole('button', { name: /Gỡ/ }))).toContain('min-h-11');
    expect(cls(screen.getAllByRole('button', { name: /Thêm máy mới/ })[0])).toContain('min-h-11');
  });

  it('bảng nằm trong khung cuộn ngang (Fleet)', async () => {
    routes['GET /api/fleet'] = () => ok({
      lead: { state: 'VACANT', agent_id: null, epoch: 0, expires_at: null, vacant_since: null, eta: null, waiting_jobs: 0 },
      workers: [{ id: 'w', name: 'w', os: 'darwin', conn_status: 'ONLINE', offline_since: null, agents: [
        { id: 'w/claude', kind: 'CLAUDE', status: 'IDLE', cooldown_until: null, lead_priority: null, soft_max_jobs: null, soft_window_hours: 5, jobs_in_window: 0, current_job: null } ] }],
    });
    routes['GET /api/projects'] = () => ok([]);
    const { container } = render(<MemoryRouter initialEntries={['/fleet']}><Fleet /></MemoryRouter>);
    await screen.findByTestId('worker-w');
    for (const t of Array.from(container.querySelectorAll('table'))) {
      let p: Element | null = t.parentElement, found = false;
      while (p && !found) { found = cls(p).includes('overflow-x-auto'); p = p.parentElement; }
      expect(found).toBe(true);
    }
  });
});
