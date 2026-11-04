// Test nghiệm thu TASK-022 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/multi-project.md §2, §4, §6; docs/03 SCR-02, SCR-03, SCR-09.
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import Kanban from './Kanban';
import TaskCreate from './TaskCreate';
import Projects from './Projects';
import ProjectSwitcher from '../components/ProjectSwitcher';
import AppShell from '../layout/AppShell';
import { ALL_PROJECTS, PROJECT_KEY, getSelectedProject, setSelectedProject } from '../project';

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
  localStorage.clear();
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
      <Route path="/projects" element={ui} />
      <Route path="/tasks/new" element={ui} />
      <Route path="/tasks/:id" element={<div>Trang chi tiết task</div>} />
    </Routes>
  </MemoryRouter>
);

const projects = [
  { id: 'p1', name: 'agenthub', github_repo: 'namth/agenthub', default_branch: 'main', status: 'ACTIVE', role: 'PARTNER', weight: 1, max_parallel: null, webhook_ok: true, open_tasks: 3 },
  { id: 'p2', name: 'edusystem', github_repo: 'namth/edusystem', default_branch: 'develop', status: 'ACTIVE', role: 'ADMIN', weight: 2, max_parallel: 4, webhook_ok: false, open_tasks: 0 },
];

// ---------- project.ts ----------

describe('lựa chọn dự án (project.ts)', () => {
  it('lưu localStorage và phát sự kiện cho nơi khác cập nhật', () => {
    expect(getSelectedProject()).toBeNull();
    const seen: string[] = [];
    const h = () => seen.push(String(getSelectedProject()));
    window.addEventListener('agenthub:project', h);
    setSelectedProject('p2');
    window.removeEventListener('agenthub:project', h);
    expect(localStorage.getItem(PROJECT_KEY)).toBe('p2');
    expect(getSelectedProject()).toBe('p2');
    expect(seen).toEqual(['p2']);
    expect(ALL_PROJECTS).toBe('*');
  });
});

// ---------- Ô chọn dự án ----------

describe('ProjectSwitcher', () => {
  it('owner: có "Tất cả dự án", mặc định chọn tất cả', async () => {
    routes['GET /api/projects'] = () => ok(projects);
    render(wrap(<ProjectSwitcher isOwner />));
    const sel = (await screen.findByLabelText('Dự án')) as HTMLSelectElement;
    await waitFor(() => expect(sel.options.length).toBe(3));
    expect(sel.options[0].value).toBe('*');
    expect(sel.options[0].textContent).toMatch(/Tất cả dự án/);
    expect(within(sel).getByText('namth/edusystem')).toBeTruthy();
    expect(sel.value).toBe('*');
    expect(getSelectedProject()).toBe('*');
  });

  it('partner: không có "Tất cả", chưa chọn → tự chọn dự án đầu tiên', async () => {
    routes['GET /api/projects'] = () => ok(projects);
    render(wrap(<ProjectSwitcher isOwner={false} />));
    const sel = (await screen.findByLabelText('Dự án')) as HTMLSelectElement;
    await waitFor(() => expect(sel.value).toBe('p1'));
    expect(Array.from(sel.options).some((o) => o.value === '*')).toBe(false);
    expect(getSelectedProject()).toBe('p1');
  });

  it('giữ lựa chọn đã lưu; lựa chọn không còn hợp lệ → về mặc định', async () => {
    routes['GET /api/projects'] = () => ok(projects);
    localStorage.setItem(PROJECT_KEY, 'p2');
    render(wrap(<ProjectSwitcher isOwner={false} />));
    const sel = (await screen.findByLabelText('Dự án')) as HTMLSelectElement;
    await waitFor(() => expect(sel.value).toBe('p2'));
    cleanup();

    localStorage.setItem(PROJECT_KEY, '*'); // partner không được xem tất cả
    render(wrap(<ProjectSwitcher isOwner={false} />));
    const sel2 = (await screen.findByLabelText('Dự án')) as HTMLSelectElement;
    await waitFor(() => expect(sel2.value).toBe('p1'));
    expect(getSelectedProject()).toBe('p1');
  });

  it('đổi lựa chọn → lưu lại', async () => {
    routes['GET /api/projects'] = () => ok(projects);
    render(wrap(<ProjectSwitcher isOwner />));
    const sel = (await screen.findByLabelText('Dự án')) as HTMLSelectElement;
    await waitFor(() => expect(sel.options.length).toBe(3));
    fireEvent.change(sel, { target: { value: 'p2' } });
    expect(getSelectedProject()).toBe('p2');
  });

  it('không có dự án nào → báo "Chưa có dự án"', async () => {
    routes['GET /api/projects'] = () => ok([]);
    render(wrap(<ProjectSwitcher isOwner={false} />));
    expect(await screen.findByText('Chưa có dự án')).toBeTruthy();
  });

  it('AppShell hiển thị ô chọn dự án trong sidebar', async () => {
    routes['GET /api/projects'] = () => ok(projects);
    render(
      <MemoryRouter>
        <Routes>
          <Route element={<AppShell user={{ id: 'u1', username: 'namth', role: 'ADMIN' } as never} officeEnabled={false} />}>
            <Route path="/" element={<div>nội dung</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );
    const aside = screen.getByRole('complementary');
    expect(await within(aside).findByLabelText('Dự án')).toBeTruthy();
  });
});

// ---------- Kanban theo dự án ----------

const board = (projectId: string) => ({
  columns: [
    { key: 'backlog', title: 'Chờ duyệt', tasks: [{ id: 't1', project_id: projectId, title: 'Xuất PDF', status: 'PENDING_APPROVAL', github_issue_id: 101, parent_id: null, created_at: '2026-11-04T09:00:00Z' }] },
    { key: 'planning', title: 'Lập kế hoạch', tasks: [] },
    { key: 'in_progress', title: 'Đang làm', tasks: [] },
    { key: 'review', title: 'Kiểm định', tasks: [] },
    { key: 'done', title: 'Xong', tasks: [] },
  ],
});

const taskGets = () => calls.filter((c) => c.method === 'GET' && c.url.startsWith('/api/tasks'));

describe('SCR-02 Kanban theo dự án', () => {
  it('đang chọn dự án → GET /api/tasks?project_id=…', async () => {
    setSelectedProject('p2');
    routes['GET /api/projects'] = () => ok(projects);
    routes['GET /api/tasks'] = () => ok(board('p2'));
    render(wrap(<Kanban role="PARTNER" />));
    await screen.findByText('Xuất PDF');
    expect(taskGets()[0].url).toBe('/api/tasks?project_id=p2');
  });

  it('đổi dự án ở nơi khác → tải lại theo dự án mới', async () => {
    setSelectedProject('p1');
    routes['GET /api/projects'] = () => ok(projects);
    routes['GET /api/tasks'] = () => ok(board('p1'));
    render(wrap(<Kanban role="PARTNER" />));
    await screen.findByText('Xuất PDF');
    act(() => setSelectedProject('p2'));
    await waitFor(() => expect(taskGets().some((c) => c.url === '/api/tasks?project_id=p2')).toBe(true));
  });

  it('"Tất cả dự án" → GET /api/tasks không tham số, thẻ ghi tên repo', async () => {
    setSelectedProject('*');
    routes['GET /api/projects'] = () => ok(projects);
    routes['GET /api/tasks'] = () => ok(board('p2'));
    render(wrap(<Kanban role="ADMIN" />));
    await screen.findByText('Xuất PDF');
    expect(taskGets()[0].url).toBe('/api/tasks');
    expect(await within(screen.getByTestId('column-backlog')).findByText('namth/edusystem')).toBeTruthy();
  });

  it('partner là ADMIN của dự án đang chọn → có nút Duyệt; PARTNER dự án → không', async () => {
    setSelectedProject('p2');
    routes['GET /api/projects'] = () => ok(projects);
    routes['GET /api/tasks'] = () => ok(board('p2'));
    render(wrap(<Kanban role="PARTNER" />));
    expect(await within(await screen.findByTestId('column-backlog')).findByRole('button', { name: 'Duyệt' })).toBeTruthy();
    cleanup();

    setSelectedProject('p1');
    render(wrap(<Kanban role="PARTNER" />));
    await screen.findByText('Xuất PDF');
    await waitFor(() => expect(calls.filter((c) => c.url === '/api/projects').length).toBeGreaterThanOrEqual(2));
    expect(screen.queryByRole('button', { name: 'Duyệt' })).toBeNull();
  });

  it('dự án đã lưu trữ → chỉ đọc (không nút Duyệt) và ghi chú', async () => {
    setSelectedProject('p2');
    routes['GET /api/projects'] = () => ok([projects[0], { ...projects[1], status: 'ARCHIVED' }]);
    routes['GET /api/tasks'] = () => ok(board('p2'));
    render(wrap(<Kanban role="ADMIN" />));
    expect(await screen.findByText(/Dự án đã lưu trữ/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Duyệt' })).toBeNull();
  });
});

// ---------- SCR-03 Tạo yêu cầu theo dự án ----------

describe('SCR-03 Tạo yêu cầu theo dự án', () => {
  const desc = 'Khách hàng cần xuất bảng tiến độ thành PDF.';
  const fill = () => {
    fireEvent.change(screen.getByLabelText('Tiêu đề'), { target: { value: 'Xuất báo cáo PDF' } });
    fireEvent.change(screen.getByLabelText('Mô tả chi tiết'), { target: { value: desc } });
    fireEvent.click(screen.getByRole('button', { name: 'Gửi yêu cầu' }));
  };

  it('đang chọn dự án → gửi kèm project_id và ghi tên dự án', async () => {
    setSelectedProject('p2');
    routes['GET /api/projects'] = () => ok(projects);
    routes['POST /api/tasks'] = () => ok({ id: 'tnew', status: 'PENDING_APPROVAL' }, 201);
    render(wrap(<TaskCreate />, '/tasks/new'));
    expect(await screen.findByText(/namth\/edusystem/)).toBeTruthy();
    fill();
    expect(await screen.findByText('Trang chi tiết task')).toBeTruthy();
    const post = calls.find((c) => c.method === 'POST')!;
    expect(post.body).toEqual({ project_id: 'p2', title: 'Xuất báo cáo PDF', description: desc });
  });

  it('"Tất cả dự án" → bắt chọn dự án trong form trước khi gửi', async () => {
    setSelectedProject('*');
    routes['GET /api/projects'] = () => ok(projects);
    routes['POST /api/tasks'] = () => ok({ id: 'tnew', status: 'PENDING_APPROVAL' }, 201);
    render(wrap(<TaskCreate />, '/tasks/new'));
    const sel = (await screen.findByLabelText('Dự án của yêu cầu')) as HTMLSelectElement;
    await waitFor(() => expect(sel.options.length).toBeGreaterThanOrEqual(2));
    fill();
    expect(await screen.findByText('Chọn dự án cho yêu cầu')).toBeTruthy();
    expect(calls.some((c) => c.method === 'POST')).toBe(false);
    fireEvent.change(sel, { target: { value: 'p1' } });
    fireEvent.click(screen.getByRole('button', { name: 'Gửi yêu cầu' }));
    await screen.findByText('Trang chi tiết task');
    expect((calls.find((c) => c.method === 'POST')!.body as { project_id: string }).project_id).toBe('p1');
  });
});

// ---------- SCR-09 Dự án ----------

const repos = [
  { full_name: 'namth/agenthub', default_branch: 'main', private: false, added: true },
  { full_name: 'namth/edusystem', default_branch: 'develop', private: true, added: true },
  { full_name: 'namth/zalo', default_branch: 'main', private: true, added: false },
];

describe('SCR-09 Dự án', () => {
  beforeEach(() => {
    routes['GET /api/projects'] = () => ok(projects);
    routes['GET /api/admin/github/repos'] = () => ok(repos);
  });

  it('đang tải → skeleton (aria-busy)', async () => {
    const { container } = render(wrap(<Projects />, '/projects'));
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    await screen.findByTestId('project-p1');
  });

  it('bảng dự án: repo, nhánh, webhook, task mở, trọng số; repo chưa thêm có nút Thêm', async () => {
    render(wrap(<Projects />, '/projects'));
    const r1 = await screen.findByTestId('project-p1');
    expect(within(r1).getByText('namth/agenthub')).toBeTruthy();
    expect(within(r1).getByText('main')).toBeTruthy();
    expect(within(r1).getByText('3')).toBeTruthy();
    const r2 = screen.getByTestId('project-p2');
    expect(within(r2).getByRole('button', { name: 'Tạo lại webhook' })).toBeTruthy();
    expect(within(r1).queryByRole('button', { name: 'Tạo lại webhook' })).toBeNull();
    const avail = screen.getByTestId('repos-available');
    expect(within(avail).getByText('namth/zalo')).toBeTruthy();
    expect(within(avail).queryByText('namth/agenthub')).toBeNull();
    expect(within(avail).getAllByRole('button', { name: 'Thêm' }).length).toBe(1);
  });

  it('Thêm repo → POST /api/admin/projects rồi tải lại; webhook lỗi → cảnh báo', async () => {
    routes['POST /api/admin/projects'] = () => ok({ id: 'p3', github_repo: 'namth/zalo', webhook_ok: false }, 201);
    render(wrap(<Projects />, '/projects'));
    const avail = await screen.findByTestId('repos-available');
    fireEvent.click(within(avail).getByRole('button', { name: 'Thêm' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/admin/projects')).toBe(true));
    expect(calls.find((c) => c.url === '/api/admin/projects')!.body).toEqual({ github_repo: 'namth/zalo' });
    expect(await screen.findByText(/webhook/i, { selector: '[role="status"], [role="alert"]' })).toBeTruthy();
    await waitFor(() => expect(calls.filter((c) => c.method === 'GET' && c.url === '/api/projects').length).toBe(2));
  });

  it('Thêm repo đã có → 409 báo lỗi', async () => {
    routes['POST /api/admin/projects'] = () => fail(409, 'CONFLICT', 'dự án đã tồn tại');
    render(wrap(<Projects />, '/projects'));
    const avail = await screen.findByTestId('repos-available');
    fireEvent.click(within(avail).getByRole('button', { name: 'Thêm' }));
    expect(await screen.findByText(/đã tồn tại/)).toBeTruthy();
  });

  it('Tạo lại webhook → POST /api/admin/projects/{id}/webhook', async () => {
    routes['POST /api/admin/projects/p2/webhook'] = () => ok({ ...projects[1], webhook_ok: true });
    render(wrap(<Projects />, '/projects'));
    fireEvent.click(within(await screen.findByTestId('project-p2')).getByRole('button', { name: 'Tạo lại webhook' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/admin/projects/p2/webhook')).toBe(true));
  });

  it('sửa trọng số / song song: validate rồi PATCH', async () => {
    routes['PATCH /api/admin/projects/p1'] = () => ok({ ...projects[0], weight: 3, max_parallel: 2 });
    render(wrap(<Projects />, '/projects'));
    const r1 = await screen.findByTestId('project-p1');
    const weight = within(r1).getByLabelText('Trọng số');
    const par = within(r1).getByLabelText('Song song tối đa');
    fireEvent.change(weight, { target: { value: '9' } });
    fireEvent.click(within(r1).getByRole('button', { name: 'Lưu' }));
    expect(await within(r1).findByText(/Trọng số 1–5/)).toBeTruthy();
    expect(calls.some((c) => c.method === 'PATCH')).toBe(false);

    fireEvent.change(weight, { target: { value: '3' } });
    fireEvent.change(par, { target: { value: '60' } });
    fireEvent.click(within(r1).getByRole('button', { name: 'Lưu' }));
    expect(await within(r1).findByText(/1–50/)).toBeTruthy();
    expect(calls.some((c) => c.method === 'PATCH')).toBe(false);

    fireEvent.change(par, { target: { value: '2' } });
    fireEvent.click(within(r1).getByRole('button', { name: 'Lưu' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true));
    expect(calls.find((c) => c.method === 'PATCH')!.body).toEqual({ weight: 3, max_parallel: 2 });
  });

  it('song song để trống → gửi max_parallel null', async () => {
    routes['PATCH /api/admin/projects/p2'] = () => ok({ ...projects[1], max_parallel: null });
    render(wrap(<Projects />, '/projects'));
    const r2 = await screen.findByTestId('project-p2');
    fireEvent.change(within(r2).getByLabelText('Song song tối đa'), { target: { value: '' } });
    fireEvent.click(within(r2).getByRole('button', { name: 'Lưu' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true));
    expect(calls.find((c) => c.method === 'PATCH')!.body).toEqual({ weight: 2, max_parallel: null });
  });

  it('Lưu trữ (có xác nhận) → PATCH status ARCHIVED; dự án lưu trữ có nút Khôi phục', async () => {
    routes['PATCH /api/admin/projects/p1'] = () => ok({ ...projects[0], status: 'ARCHIVED' });
    render(wrap(<Projects />, '/projects'));
    const r1 = await screen.findByTestId('project-p1');
    vi.stubGlobal('confirm', () => false);
    fireEvent.click(within(r1).getByRole('button', { name: 'Lưu trữ' }));
    expect(calls.some((c) => c.method === 'PATCH')).toBe(false);
    vi.stubGlobal('confirm', () => true);
    fireEvent.click(within(r1).getByRole('button', { name: 'Lưu trữ' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true));
    expect(calls.find((c) => c.method === 'PATCH')!.body).toEqual({ status: 'ARCHIVED' });
    cleanup();

    routes['GET /api/projects'] = () => ok([{ ...projects[0], status: 'ARCHIVED' }]);
    routes['PATCH /api/admin/projects/p1'] = () => ok(projects[0]);
    calls = [];
    render(wrap(<Projects />, '/projects'));
    const ar = await screen.findByTestId('project-p1');
    expect(within(ar).getByText(/Đã lưu trữ/)).toBeTruthy();
    fireEvent.click(within(ar).getByRole('button', { name: 'Khôi phục' }));
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ status: 'ACTIVE' }));
  });

  it('chưa có dự án → "Chưa có dự án" và vẫn hiện repo để thêm', async () => {
    routes['GET /api/projects'] = () => ok([]);
    routes['GET /api/admin/github/repos'] = () => ok(repos.map((r) => ({ ...r, added: false })));
    render(wrap(<Projects />, '/projects'));
    expect(await screen.findByText('Chưa có dự án')).toBeTruthy();
    expect(within(screen.getByTestId('repos-available')).getAllByRole('button', { name: 'Thêm' }).length).toBe(3);
  });

  it('PAT lỗi (502 GITHUB_ERROR) → banner nói rõ quyền PAT cần cấp, bảng dự án vẫn hiện', async () => {
    routes['GET /api/admin/github/repos'] = () => fail(502, 'GITHUB_ERROR', 'Bad credentials');
    render(wrap(<Projects />, '/projects'));
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toMatch(/PAT/);
    expect(screen.getByTestId('project-p1')).toBeTruthy();
  });

  it('lỗi tải dự án → banner Thử lại gọi lại API', async () => {
    let n = 0;
    routes['GET /api/projects'] = () => (++n === 1 ? fail(500, 'INTERNAL') : ok(projects));
    render(wrap(<Projects />, '/projects'));
    const alert = await screen.findByRole('alert');
    fireEvent.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await screen.findByTestId('project-p1');
  });
});
