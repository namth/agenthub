// Test nghiệm thu TASK-015 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/03 §1 (SCR-04, SCR-07), §3; API khoá ở internal/tasks/tasks_test.go, internal/office/office_test.go.
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import TaskDetail from './TaskDetail';
import Settings from './Settings';

type Call = { url: string; method: string; body?: unknown };
let calls: Call[] = [];
let routes: Record<string, () => { status: number; body: unknown }> = {};
const ok = (data: unknown, status = 200) => ({ status, body: { success: true, data, error: null } });
const fail = (status: number, code: string) => ({ status, body: { success: false, data: null, error: { code, message: code } } });

beforeEach(() => {
  calls = [];
  routes = {};
  vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ url: input, method, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const h = routes[`${method} ${input.split('?')[0]}`];
    const r = h ? h() : fail(404, 'NOT_FOUND');
    return new Response(JSON.stringify(r.body), { status: r.status, headers: { 'Content-Type': 'application/json' } });
  });
  vi.stubGlobal('confirm', () => true);
  vi.stubGlobal('prompt', () => 'Ngoài phạm vi hợp đồng');
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const task = {
  id: 't1', title: 'Xuất báo cáo PDF', description: 'Khách cần xuất bảng tiến độ thành PDF', status: 'PENDING_APPROVAL',
  github_issue_id: 101, pr_url: null, parent_id: null, created_at: '2026-11-04T09:00:00Z',
  subtasks: [
    { id: 's1', title: 'Schema DB', status: 'IN_PROGRESS', github_issue_id: 201, pr_url: null },
    { id: 's2', title: 'API xuất PDF', status: 'WAITING_QA', github_issue_id: 202, pr_url: 'https://github.com/namth/agenthub/pull/15' },
  ],
};

const detail = (role: 'ADMIN' | 'PARTNER') =>
  render(
    <MemoryRouter initialEntries={['/tasks/t1']}>
      <Routes>
        <Route path="/tasks/:id" element={<TaskDetail role={role} />} />
      </Routes>
    </MemoryRouter>,
  );

describe('SCR-04 Chi tiết task', () => {
  it('đang tải → aria-busy; xong → tiêu đề, mô tả, issue, sub-task và PR', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task);
    const { container } = detail('ADMIN');
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(await screen.findByRole('heading', { name: 'Xuất báo cáo PDF' })).toBeTruthy();
    expect(screen.getByText('Khách cần xuất bảng tiến độ thành PDF')).toBeTruthy();
    expect(screen.getByText(/#101/)).toBeTruthy();
    const subs = screen.getByTestId('subtasks');
    expect(within(subs).getByText('Schema DB')).toBeTruthy();
    expect(within(subs).getByRole('link', { name: /#15|PR/ }).getAttribute('href')).toBe('https://github.com/namth/agenthub/pull/15');
  });

  it('task không tồn tại → thông báo', async () => {
    routes['GET /api/tasks/t1'] = () => fail(404, 'NOT_FOUND');
    detail('ADMIN');
    expect(await screen.findByText('Không tìm thấy task')).toBeTruthy();
  });

  it('lỗi khác → banner Thử lại', async () => {
    let n = 0;
    routes['GET /api/tasks/t1'] = () => (++n === 1 ? fail(500, 'INTERNAL') : ok(task));
    detail('ADMIN');
    fireEvent.click(within(await screen.findByRole('alert')).getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByRole('heading', { name: 'Xuất báo cáo PDF' })).toBeTruthy();
  });

  it('Admin duyệt task chờ duyệt → POST approve rồi tải lại', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task);
    routes['POST /api/tasks/t1/approve'] = () => ok({ id: 't1', status: 'PLANNING' });
    detail('ADMIN');
    fireEvent.click(await screen.findByRole('button', { name: 'Duyệt' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/tasks/t1/approve')).toBe(true));
    await waitFor(() => expect(calls.filter((c) => c.method === 'GET').length).toBe(2));
  });

  it('Admin từ chối → hỏi lý do, POST reject kèm reason; huỷ thì không gọi', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task);
    routes['POST /api/tasks/t1/reject'] = () => ok({ id: 't1', status: 'REJECTED' });
    detail('ADMIN');
    const btn = await screen.findByRole('button', { name: 'Từ chối' });
    vi.stubGlobal('prompt', () => null);
    fireEvent.click(btn);
    expect(calls.some((c) => c.url.endsWith('/reject'))).toBe(false);
    vi.stubGlobal('prompt', () => 'Ngoài phạm vi hợp đồng');
    fireEvent.click(btn);
    await waitFor(() => expect(calls.find((c) => c.url === '/api/tasks/t1/reject')?.body).toEqual({ reason: 'Ngoài phạm vi hợp đồng' }));
  });

  it('Partner không có nút Duyệt/Từ chối; task đã duyệt cũng không', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task);
    detail('PARTNER');
    await screen.findByRole('heading', { name: 'Xuất báo cáo PDF' });
    expect(screen.queryByRole('button', { name: 'Duyệt' })).toBeNull();
    cleanup();
    routes['GET /api/tasks/t1'] = () => ok({ ...task, status: 'PLANNING' });
    detail('ADMIN');
    await screen.findByRole('heading', { name: 'Xuất báo cáo PDF' });
    expect(screen.queryByRole('button', { name: 'Duyệt' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Từ chối' })).toBeNull();
  });
});

describe('SCR-07 Cài đặt', () => {
  it('đọc và bật văn phòng ảo → PATCH, báo onOfficeChange', async () => {
    routes['GET /api/admin/settings'] = () => ok({ office_enabled: false });
    routes['PATCH /api/admin/settings'] = () => ok({ office_enabled: true });
    const changes: boolean[] = [];
    render(<MemoryRouter><Settings onOfficeChange={(v) => changes.push(v)} /></MemoryRouter>);
    const box = (await screen.findByRole('checkbox', { name: /Bật văn phòng ảo/ })) as HTMLInputElement;
    expect(box.checked).toBe(false);
    fireEvent.click(box);
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ office_enabled: true }));
    await waitFor(() => expect(changes).toEqual([true]));
    expect(box.checked).toBe(true);
  });

  it('PATCH lỗi → trả lại trạng thái cũ và báo lỗi', async () => {
    routes['GET /api/admin/settings'] = () => ok({ office_enabled: true });
    routes['PATCH /api/admin/settings'] = () => fail(500, 'INTERNAL');
    const changes: boolean[] = [];
    render(<MemoryRouter><Settings onOfficeChange={(v) => changes.push(v)} /></MemoryRouter>);
    const box = (await screen.findByRole('checkbox', { name: /Bật văn phòng ảo/ })) as HTMLInputElement;
    fireEvent.click(box);
    expect(await screen.findByRole('alert')).toBeTruthy();
    expect(box.checked).toBe(true);
    expect(changes).toEqual([]);
  });
});
