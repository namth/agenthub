// Test nghiệm thu TASK-040 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/clarification-and-shared-skills.md §4, §6; TC-82.
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import TaskDetail from './TaskDetail';
import Kanban from './Kanban';

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
  vi.stubGlobal('confirm', () => true);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const task = (status = 'WAITING_USER') => ({
  id: 't1', title: 'Tích hợp thanh toán', description: 'Khách muốn thanh toán online trên web', status,
  github_issue_id: 105, pr_url: null, parent_id: null, created_at: '2026-11-04T09:00:00Z', subtasks: [],
});
const waiting = {
  round: 2, status: 'WAITING', answers: null, answered_by: null, answered_via: null, answered_at: null,
  questions: [
    { id: 'payment_gateway', question: 'Cần hỗ trợ cổng nào trước?', options: ['VNPay', 'MoMo', 'Stripe'], recommended: 'VNPay', reason: 'Phổ biến trong nước' },
    { id: 'refund_scope', question: 'Có cần hoàn tiền tự động không?', options: [], recommended: '', reason: 'Ảnh hưởng thiết kế bảng giao dịch' },
  ],
};
const answered = {
  round: 1, status: 'ANSWERED', answered_by: 'doi-tac', answered_via: 'GITHUB', answered_at: '2026-11-04T10:00:00Z',
  questions: [{ id: 'audience', question: 'Ai sẽ thanh toán?', options: ['Khách lẻ', 'Doanh nghiệp'], recommended: 'Khách lẻ', reason: '' }],
  answers: { audience: 'Khách lẻ' },
};
const clar = (over: Record<string, unknown> = {}) => ({ rounds: [answered, waiting], can_answer: true, can_auto: true, ...over });

const page = (role: 'ADMIN' | 'PARTNER' = 'PARTNER') =>
  render(
    <MemoryRouter initialEntries={['/tasks/t1']}>
      <Routes>
        <Route path="/tasks/:id" element={<TaskDetail role={role} />} />
      </Routes>
    </MemoryRouter>,
  );

describe('SCR-04 phần Làm rõ', () => {
  it('hiển thị băng, câu hỏi, phương án (Đề xuất), lịch sử vòng trước', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task());
    routes['GET /api/tasks/t1/clarifications'] = () => ok(clar());
    page();
    const sec = await screen.findByTestId('clarify');
    expect(within(sec).getByRole('status').textContent).toMatch(/Trưởng phòng cần bạn trả lời/);
    expect(within(sec).getByRole('group', { name: 'Cần hỗ trợ cổng nào trước?' })).toBeTruthy();
    expect(within(sec).getByLabelText('VNPay (Đề xuất)')).toBeTruthy();
    expect(within(sec).getByLabelText('MoMo')).toBeTruthy();
    expect(within(sec).getByText('Phổ biến trong nước')).toBeTruthy();
    expect(within(sec).getByLabelText('Khác: Có cần hoàn tiền tự động không?')).toBeTruthy();
    const hist = within(sec).getByTestId('clarify-history');
    expect(hist.textContent).toMatch(/Ai sẽ thanh toán\?/);
    expect(hist.textContent).toMatch(/Khách lẻ/);
    expect(hist.textContent).toMatch(/doi-tac/);
  });

  it('thiếu câu trả lời → báo lỗi, không gọi API', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task());
    routes['GET /api/tasks/t1/clarifications'] = () => ok(clar());
    page();
    const sec = await screen.findByTestId('clarify');
    fireEvent.click(within(sec).getByLabelText('VNPay (Đề xuất)'));
    fireEvent.click(within(sec).getByRole('button', { name: 'Gửi câu trả lời' }));
    expect((await within(sec).findByRole('alert')).textContent).toMatch(/Hãy trả lời đủ/);
    expect(calls.some((c) => c.method === 'POST')).toBe(false);
  });

  it('gửi đủ → POST đúng body, trạng thái đang gửi, thông báo thành công, tải lại', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task());
    routes['GET /api/tasks/t1/clarifications'] = () => ok(clar());
    let release!: () => void;
    const gate = new Promise<void>((r) => { release = r; });
    routes['POST /api/tasks/t1/clarify'] = () => ok({ status: 'PLANNING' });
    const origFetch = globalThis.fetch;
    vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
      if ((init?.method ?? 'GET').toUpperCase() === 'POST') await gate;
      return origFetch(input, init);
    });
    page();
    const sec = await screen.findByTestId('clarify');
    fireEvent.click(within(sec).getByLabelText('MoMo'));
    fireEvent.change(within(sec).getByLabelText('Khác: Có cần hoàn tiền tự động không?'), { target: { value: 'Có, trong 7 ngày' } });
    fireEvent.change(within(sec).getByLabelText('Ghi chú thêm'), { target: { value: 'Ưu tiên mobile' } });
    fireEvent.click(within(sec).getByRole('button', { name: 'Gửi câu trả lời' }));
    const busy = await within(sec).findByRole('button', { name: /Đang gửi/ });
    expect((busy as HTMLButtonElement).disabled).toBe(true);
    await act(async () => { release(); });
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/tasks/t1/clarify')).toBe(true));
    expect(calls.find((c) => c.method === 'POST')!.body).toEqual({
      answers: { payment_gateway: 'MoMo', refund_scope: 'Có, trong 7 ngày', _notes: 'Ưu tiên mobile' },
    });
    expect(await screen.findByText(/Đã gửi — trưởng phòng đang hoàn thiện kế hoạch/)).toBeTruthy();
    await waitFor(() => expect(calls.filter((c) => c.url === '/api/tasks/t1/clarifications').length).toBeGreaterThanOrEqual(2));
  });

  it('409 → báo đã được trả lời; lỗi khác giữ nội dung đã nhập', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task());
    routes['GET /api/tasks/t1/clarifications'] = () => ok(clar());
    let n = 0;
    routes['POST /api/tasks/t1/clarify'] = () => (++n === 1 ? fail(500, 'INTERNAL', 'lỗi máy chủ') : fail(409, 'INVALID_TRANSITION', 'đã được trả lời bởi namth'));
    page();
    const sec = await screen.findByTestId('clarify');
    fireEvent.click(within(sec).getByLabelText('VNPay (Đề xuất)'));
    const other = within(sec).getByLabelText('Khác: Có cần hoàn tiền tự động không?') as HTMLInputElement;
    fireEvent.change(other, { target: { value: 'Không' } });
    fireEvent.click(within(sec).getByRole('button', { name: 'Gửi câu trả lời' }));
    expect(await within(sec).findByRole('alert')).toBeTruthy();
    expect((within(sec).getByLabelText('Khác: Có cần hoàn tiền tự động không?') as HTMLInputElement).value).toBe('Không');
    fireEvent.click(within(sec).getByRole('button', { name: 'Gửi câu trả lời' }));
    await waitFor(() => expect(within(screen.getByTestId('clarify')).getByRole('alert').textContent).toMatch(/đã được trả lời/));
  });

  it('Cho trưởng phòng tự quyết (can_auto) → POST clarify/auto; ẩn khi không có quyền', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task());
    routes['GET /api/tasks/t1/clarifications'] = () => ok(clar());
    routes['POST /api/tasks/t1/clarify/auto'] = () => ok({ status: 'PLANNING' });
    page('ADMIN');
    const sec = await screen.findByTestId('clarify');
    fireEvent.click(within(sec).getByRole('button', { name: 'Cho trưởng phòng tự quyết' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/tasks/t1/clarify/auto')).toBe(true));
    cleanup();
    routes['GET /api/tasks/t1/clarifications'] = () => ok(clar({ can_auto: false, can_answer: false }));
    page();
    const ro = await screen.findByTestId('clarify');
    expect(within(ro).queryByRole('button', { name: 'Cho trưởng phòng tự quyết' })).toBeNull();
    expect(within(ro).queryByRole('button', { name: 'Gửi câu trả lời' })).toBeNull();
    expect((within(ro).getByLabelText('VNPay (Đề xuất)') as HTMLInputElement).disabled).toBe(true);
  });

  it('không có vòng làm rõ → không hiện phần làm rõ', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task('PLANNING'));
    routes['GET /api/tasks/t1/clarifications'] = () => ok({ rounds: [], can_answer: false, can_auto: false });
    page();
    await screen.findByRole('heading', { name: 'Tích hợp thanh toán' });
    await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
    expect(screen.queryByTestId('clarify')).toBeNull();
  });

  it('nội dung câu hỏi không được hiểu là HTML', async () => {
    routes['GET /api/tasks/t1'] = () => ok(task());
    routes['GET /api/tasks/t1/clarifications'] = () => ok({ rounds: [{ ...waiting, questions: [{ id: 'x', question: '<img src=x onerror=alert(1)>Câu?', options: ['A'], recommended: 'A', reason: '<b>đậm</b>' }] }], can_answer: true, can_auto: false });
    const { container } = page();
    await screen.findByTestId('clarify');
    expect(container.querySelector('img[src="x"]')).toBeNull();
    expect(container.querySelector('#root b, [data-testid="clarify"] b')).toBeNull();
  });
});

describe('Kanban nhãn Cần làm rõ', () => {
  it('thẻ WAITING_USER ở cột Planning có nhãn số câu', async () => {
    routes['GET /api/projects'] = () => ok([]);
    routes['GET /api/tasks'] = () => ok({ columns: [
      { key: 'backlog', title: 'Chờ duyệt', tasks: [] },
      { key: 'planning', title: 'Lập kế hoạch', tasks: [{ id: 't1', title: 'Tích hợp thanh toán', status: 'WAITING_USER', clarify_pending: 2, github_issue_id: 105, parent_id: null, created_at: '2026-11-04T09:00:00Z' }] },
      { key: 'in_progress', title: 'Đang làm', tasks: [] }, { key: 'review', title: 'Kiểm định', tasks: [] }, { key: 'done', title: 'Xong', tasks: [] },
    ] });
    render(<MemoryRouter><Kanban role="ADMIN" /></MemoryRouter>);
    const col = await screen.findByTestId('column-planning');
    expect(within(col).getByText('Cần làm rõ (2 câu)')).toBeTruthy();
  });
});
