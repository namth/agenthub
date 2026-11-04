// Test nghiệm thu TASK-025 — do trưởng phòng viết, file bị khoá (tests.lock). SCR-10, DEC-21.
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import Contributions from './Contributions';

let calls: string[] = [];
const rows = {
  partner: [
    { key: 'u-owner', label: 'namth', machines: 2, jobs: { done: 41, failed: 1, exhausted: 2, by_kind: { CODE: 30, QA_REVIEW: 8, SPEC: 3 } }, tasks_done: 6, prs_merged: 6, busy_hours: 11.5 },
    { key: 'u-a', label: 'doi-tac-a', machines: 1, jobs: { done: 12, failed: 0, exhausted: 1, by_kind: { CODE: 12 } }, tasks_done: 2, prs_merged: 2, busy_hours: 3.2 },
  ],
  project: [{ key: 'p1', label: 'namth/agenthub', machines: 3, jobs: { done: 53, failed: 1, exhausted: 3, by_kind: { CODE: 42 } }, tasks_done: 8, prs_merged: 8, busy_hours: 14.7 }],
};
const timeline = [{ at: '2026-11-04T10:21:00Z', worker: 'macbook', agent: 'macbook/antigravity', job_kind: 'CODE', task_id: 't1', task_title: 'Schema DB', project: 'namth/agenthub', result: 'SUBMITTED', attempt: 1 }];

beforeEach(() => {
  calls = [];
  vi.stubGlobal('fetch', async (url: string) => {
    calls.push(url);
    const u = new URL(url, 'http://x');
    let data: unknown = [];
    if (u.pathname === '/api/contributions') data = rows[(u.searchParams.get('by') as 'partner' | 'project') ?? 'partner'];
    if (u.pathname === '/api/contributions/timeline') data = timeline;
    return new Response(JSON.stringify({ success: true, data, error: null }), { status: 200 });
  });
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const page = () => render(<MemoryRouter><Contributions /></MemoryRouter>);

describe('SCR-10 Đóng góp', () => {
  it('mặc định 7 ngày theo partner, hiện bảng tổng hợp', async () => {
    page();
    const table = await screen.findByRole('table');
    expect(within(table).getByText('namth')).toBeTruthy();
    expect(within(table).getByText('doi-tac-a')).toBeTruthy();
    expect(within(table).getByText('41')).toBeTruthy();
    expect(within(table).getByText('11.5')).toBeTruthy();
    const u = new URL(calls[0], 'http://x');
    expect(u.searchParams.get('by')).toBe('partner');
    const days = (new Date(u.searchParams.get('to')!).getTime() - new Date(u.searchParams.get('from')!).getTime()) / 86400000;
    expect(Math.round(days)).toBe(7);
  });

  it('đổi sang theo dự án và khoảng 30 ngày', async () => {
    page();
    await screen.findByRole('table');
    fireEvent.click(screen.getByRole('radio', { name: 'Dự án' }));
    expect(await screen.findByText('namth/agenthub')).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Khoảng thời gian'), { target: { value: '30' } });
    await waitFor(() => {
      const u = new URL(calls[calls.length - 1], 'http://x');
      const days = (new Date(u.searchParams.get('to')!).getTime() - new Date(u.searchParams.get('from')!).getTime()) / 86400000;
      expect(u.searchParams.get('by')).toBe('project');
      expect(Math.round(days)).toBe(30);
    });
  });

  it('bấm một dòng → dòng thời gian chi tiết', async () => {
    page();
    fireEvent.click(await screen.findByText('doi-tac-a'));
    expect(await screen.findByText('Schema DB')).toBeTruthy();
    expect(screen.getByText(/macbook\/antigravity/)).toBeTruthy();
    const t = calls.find((c) => c.startsWith('/api/contributions/timeline'))!;
    expect(new URL(t, 'http://x').searchParams.get('partner')).toBe('u-a');
  });

  it('rỗng → thông báo', async () => {
    rows.partner.length = 0;
    page();
    expect(await screen.findByText('Chưa có đóng góp trong khoảng này')).toBeTruthy();
  });
});
