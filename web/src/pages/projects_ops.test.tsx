// Test nghiệm thu TASK-044 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/ops-readiness.md §4 (SCR-09: lệnh được phép, CI, sandbox, Chạy thử); TC-89.
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import Projects from './Projects';

type Call = { url: string; method: string; body?: unknown };
let calls: Call[] = [];
let routes: Record<string, () => { status: number; body: unknown }> = {};
const ok = (data: unknown, status = 200) => ({ status, body: { success: true, data, error: null } });
const fail = (status: number, code: string, message = code) => ({ status, body: { success: false, data: null, error: { code, message } } });

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
  routes['GET /api/admin/github/repos'] = () => ok([]);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const base = { default_branch: 'main', status: 'ACTIVE', role: 'ADMIN', weight: 1, max_parallel: null, webhook_ok: true, open_tasks: 0 };
const projects = (sandbox: boolean) => [
  { ...base, id: 'p1', name: 'agenthub', github_repo: 'namth/agenthub', allowed_commands: ['go test', 'gofmt'], needs_ci_for_integration: true, is_sandbox: false },
  { ...base, id: 'sb', name: 'sandbox', github_repo: 'namth/agenthub-sandbox', allowed_commands: [], needs_ci_for_integration: false, is_sandbox: sandbox },
];
const page = () => render(<MemoryRouter initialEntries={['/projects']}><Routes><Route path="/projects" element={<Projects />} /></Routes></MemoryRouter>);

describe('SCR-09 cấu hình vận hành dự án', () => {
  it('hiện lệnh được phép, công tắc CI và sandbox, danh sách mặc định', async () => {
    routes['GET /api/projects'] = () => ok(projects(true));
    page();
    const box = await screen.findByTestId('project-ops-p1');
    expect((within(box).getByLabelText('Lệnh được phép') as HTMLTextAreaElement).value).toBe('go test\ngofmt');
    expect((within(box).getByLabelText('Cần CI cho test tích hợp') as HTMLInputElement).checked).toBe(true);
    expect((within(box).getByLabelText('Dự án sandbox') as HTMLInputElement).checked).toBe(false);
    expect(box.textContent).toMatch(/Mặc định: git, gh pr create, gh pr view/);
    expect(within(box).queryByRole('button', { name: 'Chạy thử dây chuyền' })).toBeNull();
  });

  it('Lưu cấu hình → PATCH dòng đã trim, bỏ dòng rỗng; 400 → báo lỗi', async () => {
    routes['GET /api/projects'] = () => ok(projects(false));
    let n = 0;
    routes['PATCH /api/admin/projects/p1'] = () => (++n === 1 ? ok({}) : fail(400, 'VALIDATION_ERROR', 'lệnh không hợp lệ: sudo'));
    page();
    const box = await screen.findByTestId('project-ops-p1');
    fireEvent.change(within(box).getByLabelText('Lệnh được phép'), { target: { value: ' go test \n\nnpx vitest\n' } });
    fireEvent.click(within(box).getByLabelText('Dự án sandbox'));
    fireEvent.click(within(box).getByRole('button', { name: 'Lưu cấu hình' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true));
    expect(calls.find((c) => c.method === 'PATCH')!.body).toEqual({ allowed_commands: ['go test', 'npx vitest'], needs_ci_for_integration: true, is_sandbox: true });
    fireEvent.change(within(box).getByLabelText('Lệnh được phép'), { target: { value: 'sudo rm' } });
    fireEvent.click(within(box).getByRole('button', { name: 'Lưu cấu hình' }));
    expect((await within(box).findByRole('alert')).textContent).toMatch(/sudo/);
  });

  it('dự án sandbox: chưa chạy → thông điệp; bấm Chạy thử → POST, khoá nút, thăm dò tới khi xong', async () => {
    routes['GET /api/projects'] = () => ok(projects(true));
    let runs: unknown[] = [];
    routes['GET /api/admin/projects/sb/smoke-runs'] = () => ok(runs);
    routes['POST /api/admin/projects/sb/smoke-run'] = () => {
      runs = [{ id: 'r1', status: 'RUNNING', started_at: '2026-11-05T09:00:00Z', ended_at: null, steps: [
        { step: 'SPEC', ok: true, agent_id: 'mac/claude', started_at: '2026-11-05T09:00:00Z', ended_at: '2026-11-05T09:01:12Z', note: '' },
        { step: 'CODE', ok: null, agent_id: '', note: '' }, { step: 'QA', ok: null, agent_id: '', note: '' }] }];
      return ok({ run_id: 'r1' }, 202);
    };
    page();
    const box = await screen.findByTestId('project-ops-sb');
    expect(await within(box).findByText('Chưa chạy thử lần nào')).toBeTruthy();
    vi.useFakeTimers({ shouldAdvanceTime: true });
    fireEvent.click(within(box).getByRole('button', { name: 'Chạy thử dây chuyền' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/admin/projects/sb/smoke-run')).toBe(true));
    const table = await within(box).findByTestId('smoke-runs-sb');
    expect(table.textContent).toMatch(/RUNNING/);
    expect(table.textContent).toMatch(/SPEC/);
    expect(table.textContent).toMatch(/mac\/claude/);
    expect((within(box).getByRole('button', { name: 'Chạy thử dây chuyền' }) as HTMLButtonElement).disabled).toBe(true);
    const before = calls.filter((c) => c.url === '/api/admin/projects/sb/smoke-runs').length;
    runs = [{ ...(runs[0] as object), status: 'PASSED', ended_at: '2026-11-05T09:06:00Z' }];
    await act(async () => { vi.advanceTimersByTime(3500); });
    await waitFor(() => expect(within(screen.getByTestId('smoke-runs-sb')).getByText(/PASSED/)).toBeTruthy());
    expect(calls.filter((c) => c.url === '/api/admin/projects/sb/smoke-runs').length).toBeGreaterThan(before);
    await waitFor(() => expect((within(screen.getByTestId('project-ops-sb')).getByRole('button', { name: 'Chạy thử dây chuyền' }) as HTMLButtonElement).disabled).toBe(false));
  });
});
