// Test nghiệm thu TASK-048 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/remote-command.md §4 (băng đề xuất huỷ SCR-04), §6.
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import TaskDetail from './TaskDetail';

type Call = { url: string; method: string };
let calls: Call[] = [];
let routes: Record<string, () => { status: number; body: unknown }> = {};
const ok = (data: unknown) => ({ status: 200, body: { success: true, data, error: null } });

beforeEach(() => {
  calls = [];
  routes = {};
  vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ url: input, method });
    const h = routes[`${method} ${input.split('?')[0]}`];
    const r = h ? h() : { status: 404, body: { success: false, data: null, error: { code: 'NOT_FOUND', message: '' } } };
    return new Response(JSON.stringify(r.body), { status: r.status, headers: { 'Content-Type': 'application/json' } });
  });
  vi.stubGlobal('confirm', () => true);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

let proposals: unknown[] = [];
const task = () => ({ id: 't-p', title: 'Báo cáo trùng', description: 'mô tả đủ dài để hợp lệ', status: 'PLANNING', github_issue_id: 120, pr_url: null, parent_id: null, created_at: '2026-11-05T09:00:00Z', subtasks: [], proposals });
const page = (role: 'ADMIN' | 'PARTNER') =>
  render(<MemoryRouter initialEntries={['/tasks/t-p']}><Routes><Route path="/tasks/:id" element={<TaskDetail role={role} />} /></Routes></MemoryRouter>);

describe('SCR-04 đề xuất huỷ của trưởng phòng AI', () => {
  beforeEach(() => {
    proposals = [{ id: 'lp1', kind: 'CANCEL', reason: 'Trùng yêu cầu #98', proposed_by: 'm/claude', status: 'PENDING', created_at: '2026-11-05T09:00:00Z' }];
    routes['GET /api/tasks/t-p'] = () => ok(task());
    routes['GET /api/tasks/t-p/clarifications'] = () => ok({ rounds: [], can_answer: false, can_auto: false });
  });

  it('ADMIN: băng lý do + Đồng ý huỷ → POST accept rồi tải lại', async () => {
    routes['POST /api/tasks/t-p/proposals/lp1/accept'] = () => { proposals = []; return ok({}); };
    page('ADMIN');
    const b = await screen.findByTestId('lead-proposal');
    expect(b.textContent).toMatch(/Trưởng phòng đề xuất huỷ: Trùng yêu cầu #98/);
    fireEvent.click(within(b).getByRole('button', { name: 'Đồng ý huỷ' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/tasks/t-p/proposals/lp1/accept')).toBe(true));
    await waitFor(() => expect(screen.queryByTestId('lead-proposal')).toBeNull());
  });

  it('ADMIN: Từ chối → POST reject', async () => {
    routes['POST /api/tasks/t-p/proposals/lp1/reject'] = () => { proposals = []; return ok({}); };
    page('ADMIN');
    fireEvent.click(within(await screen.findByTestId('lead-proposal')).getByRole('button', { name: 'Từ chối' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/tasks/t-p/proposals/lp1/reject')).toBe(true));
  });

  it('PARTNER: chỉ xem, không có nút', async () => {
    page('PARTNER');
    const b = await screen.findByTestId('lead-proposal');
    expect(within(b).queryByRole('button')).toBeNull();
  });

  it('không có đề xuất → không hiện băng', async () => {
    proposals = [];
    page('ADMIN');
    await screen.findByRole('heading', { name: 'Báo cáo trùng' });
    expect(screen.queryByTestId('lead-proposal')).toBeNull();
  });
});
