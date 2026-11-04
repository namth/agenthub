// Test do trưởng phòng viết: nút Merge trên SCR-04 (TASK-018, docs/12 QA_PASSED → DONE).
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import TaskDetail from './TaskDetail';

let calls: { url: string; method: string }[] = [];
let status = 'QA_PASSED';
let mergeStatus = 200;

beforeEach(() => {
  calls = [];
  status = 'QA_PASSED';
  mergeStatus = 200;
  vi.stubGlobal('confirm', () => true);
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ url, method });
    if (method === 'POST' && url === '/api/tasks/s1/merge') {
      if (mergeStatus !== 200)
        return new Response(JSON.stringify({ success: false, data: null, error: { code: 'GITHUB_ERROR', message: 'PR chưa merge được' } }), { status: mergeStatus });
      status = 'DONE';
      return new Response(JSON.stringify({ success: true, data: null, error: null }), { status: 200 });
    }
    return new Response(JSON.stringify({ success: true, error: null, data: {
      id: 's1', title: 'Schema DB', description: 'x', status, github_issue_id: 201,
      pr_url: 'https://github.com/namth/agenthub/pull/15', parent_id: 't1', created_at: '', subtasks: [] } }), { status: 200 });
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const page = (role: 'ADMIN' | 'PARTNER') =>
  render(<MemoryRouter initialEntries={['/tasks/s1']}><Routes><Route path="/tasks/:id" element={<TaskDetail role={role} />} /></Routes></MemoryRouter>);

describe('SCR-04 nút Merge', () => {
  it('Admin merge task QA_PASSED → POST merge rồi tải lại thành DONE', async () => {
    page('ADMIN');
    fireEvent.click(await screen.findByRole('button', { name: 'Merge vào main' }));
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/tasks/s1/merge')).toBe(true));
    expect(await screen.findByText('Hoàn thành')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Merge vào main' })).toBeNull();
  });

  it('merge lỗi → báo lỗi, vẫn còn nút', async () => {
    mergeStatus = 502;
    page('ADMIN');
    fireEvent.click(await screen.findByRole('button', { name: 'Merge vào main' }));
    expect(await screen.findByRole('alert')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Merge vào main' })).toBeTruthy();
  });

  it('Partner hoặc task chưa QA_PASSED: không có nút', async () => {
    page('PARTNER');
    await screen.findByText('Schema DB');
    expect(screen.queryByRole('button', { name: 'Merge vào main' })).toBeNull();
    cleanup();
    status = 'WAITING_QA';
    page('ADMIN');
    await screen.findByText('Schema DB');
    expect(screen.queryByRole('button', { name: 'Merge vào main' })).toBeNull();
  });
});
