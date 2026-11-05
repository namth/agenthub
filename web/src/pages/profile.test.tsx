// Test nghiệm thu TASK-046 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/remote-command.md §4 (SCR-12), §6; TC-92.
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import Profile from './Profile';
import { visibleNav } from '../nav';

type Call = { url: string; method: string; body?: unknown };
let calls: Call[] = [];
let routes: Record<string, () => { status: number; body: unknown }> = {};
let clip: string[] = [];
const ok = (data: unknown, status = 200) => ({ status, body: { success: true, data, error: null } });
const fail = (status: number, code: string) => ({ status, body: { success: false, data: null, error: { code, message: code } } });

beforeEach(() => {
  calls = [];
  routes = {};
  clip = [];
  vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ url: input, method, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const h = routes[`${method} ${input.split('?')[0]}`];
    const r = h ? h() : fail(404, 'NOT_FOUND');
    if (r.status === 204) return new Response(null, { status: 204 });
    return new Response(JSON.stringify(r.body), { status: r.status, headers: { 'Content-Type': 'application/json' } });
  });
  vi.stubGlobal('confirm', () => true);
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async (s: string) => { clip.push(s); } } });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const owner = { id: 'u1', github_id: '1', username: 'namth', avatar_url: '', role: 'ADMIN' as const };
const partner = { ...owner, id: 'u2', username: 'doi-tac', role: 'PARTNER' as const };
const tok = { id: 'k1', name: 'laptop', last4: 'AbCd', expires_at: '2027-02-03T09:00:00Z', last_used_at: null, created_at: '2026-11-05T09:00:00Z' };

describe('SCR-12 Hồ sơ', () => {
  it('có trong điều hướng cho mọi vai', () => {
    expect(visibleNav('ADMIN', false).some((n) => n.code === 'SCR-12' && n.path === '/profile')).toBe(true);
    expect(visibleNav('PARTNER', false).some((n) => n.code === 'SCR-12')).toBe(true);
  });

  it('owner: danh sách rỗng → hướng dẫn + nút Tạo token', async () => {
    routes['GET /api/me/tokens'] = () => ok([]);
    render(<MemoryRouter><Profile user={owner} /></MemoryRouter>);
    expect(screen.getByText('namth')).toBeTruthy();
    expect(await screen.findByText('Chưa có token')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Tạo token' })).toBeTruthy();
  });

  it('tạo token → hiện token một lần, sao chép, đoạn cấu hình MCP', async () => {
    let list: unknown[] = [];
    routes['GET /api/me/tokens'] = () => ok(list);
    routes['POST /api/me/tokens'] = () => { list = [tok]; return ok({ ...tok, token: 'ahp_GIA_TRI_BI_MAT_1234567890AbCd' }, 201); };
    render(<MemoryRouter><Profile user={owner} /></MemoryRouter>);
    await screen.findByText('Chưa có token');
    fireEvent.change(screen.getByLabelText('Tên token'), { target: { value: 'laptop' } });
    fireEvent.change(screen.getByLabelText('Hết hạn'), { target: { value: '90' } });
    fireEvent.click(screen.getByRole('button', { name: 'Tạo token' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true));
    expect(calls.find((c) => c.method === 'POST')!.body).toEqual({ name: 'laptop', expires_days: 90 });
    const box = await screen.findByTestId('new-token');
    expect(box.textContent).toContain('ahp_GIA_TRI_BI_MAT_1234567890AbCd');
    expect(box.textContent).toMatch(/không hiển thị lại/);
    expect(box.textContent).toMatch(/claude mcp add --transport http agenthub .*\/mcp --header "Authorization: Bearer ahp_GIA_TRI_BI_MAT_1234567890AbCd"/);
    fireEvent.click(within(box).getAllByRole('button', { name: /Sao chép/ })[0]);
    await waitFor(() => expect(clip.some((s) => s.includes('ahp_GIA_TRI_BI_MAT_1234567890AbCd'))).toBe(true));
    const row = await screen.findByTestId('token-k1');
    expect(row.textContent).toContain('laptop');
    expect(row.textContent).toContain('AbCd');
    expect(row.textContent).not.toContain('ahp_GIA_TRI_BI_MAT');
  });

  it('tên rỗng → không gọi API; lỗi 409 hiện ở form', async () => {
    routes['GET /api/me/tokens'] = () => ok([]);
    routes['POST /api/me/tokens'] = () => fail(409, 'CONFLICT');
    render(<MemoryRouter><Profile user={owner} /></MemoryRouter>);
    await screen.findByText('Chưa có token');
    fireEvent.click(screen.getByRole('button', { name: 'Tạo token' }));
    expect(calls.some((c) => c.method === 'POST')).toBe(false);
    fireEvent.change(screen.getByLabelText('Tên token'), { target: { value: 'trung' } });
    fireEvent.click(screen.getByRole('button', { name: 'Tạo token' }));
    expect(await screen.findByRole('alert')).toBeTruthy();
  });

  it('thu hồi → DELETE rồi tải lại', async () => {
    let list: unknown[] = [tok];
    routes['GET /api/me/tokens'] = () => ok(list);
    routes['DELETE /api/me/tokens/k1'] = () => { list = []; return { status: 204, body: null }; };
    render(<MemoryRouter><Profile user={owner} /></MemoryRouter>);
    fireEvent.click(within(await screen.findByTestId('token-k1')).getByRole('button', { name: 'Thu hồi' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url === '/api/me/tokens/k1')).toBe(true));
    expect(await screen.findByText('Chưa có token')).toBeTruthy();
  });

  it('partner: không có phần token, không gọi API token', async () => {
    render(<MemoryRouter><Profile user={partner} /></MemoryRouter>);
    expect(screen.getByText('doi-tac')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Tạo token' })).toBeNull();
    await new Promise((r) => setTimeout(r, 30));
    expect(calls.some((c) => c.url.startsWith('/api/me/tokens'))).toBe(false);
  });
});
