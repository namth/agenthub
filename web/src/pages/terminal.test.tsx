// Test nghiệm thu TASK-017 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/03 §1 SCR-06, §3; luồng SSE theo internal/logs/logs_test.go (history, log).
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import LiveTerminal from './LiveTerminal';
import { ansiToSegments } from '../components/ansi';

class FakeES {
  static last: FakeES | null = null;
  url: string;
  onerror: ((e: unknown) => void) | null = null;
  onopen: ((e: unknown) => void) | null = null;
  listeners: Record<string, ((e: MessageEvent) => void)[]> = {};
  closed = false;
  constructor(url: string) { this.url = url; FakeES.last = this; }
  addEventListener(t: string, fn: (e: MessageEvent) => void) { (this.listeners[t] ??= []).push(fn); }
  removeEventListener() {}
  close() { this.closed = true; }
  emit(t: string, d: unknown) { for (const fn of this.listeners[t] ?? []) fn(new MessageEvent(t, { data: JSON.stringify(d) })); }
}

beforeEach(() => { FakeES.last = null; vi.stubGlobal('EventSource', FakeES); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const page = () => render(
  <MemoryRouter initialEntries={['/fleet/mac-mini/terminal']}>
    <Routes><Route path="/fleet/:id/terminal" element={<LiveTerminal />} /></Routes>
  </MemoryRouter>,
);
const line = (id: number, chunk: string, aid = 'asg_1', task = 't1') => ({ id, worker_id: 'mac-mini', assignment_id: aid, task_id: task, chunk, ts: '2026-11-04T14:00:00Z' });

describe('ansiToSegments', () => {
  it('tách màu SGR cơ bản và reset', () => {
    const s = ansiToSegments('\x1b[32mok\x1b[0m thường \x1b[1;31mlỗi\x1b[0m');
    expect(s.map((x) => x.text)).toEqual(['ok', ' thường ', 'lỗi']);
    expect(s[0].color).toBe('green');
    expect(s[1].color).toBeUndefined();
    expect(s[2].color).toBe('red');
    expect(s[2].bold).toBe(true);
  });
  it('bỏ mã điều khiển lạ, không lộ ký tự ESC', () => {
    const s = ansiToSegments('a\x1b[2Kb\x1b[?25lc');
    expect(s.map((x) => x.text).join('')).toBe('abc');
  });
});

describe('SCR-06 Live terminal', () => {
  it('mở luồng log của đúng máy; đang tải → aria-busy', () => {
    const { container } = page();
    expect(FakeES.last?.url).toBe('/api/admin/workers/mac-mini/logs/stream');
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(screen.getByText(/mac-mini/)).toBeTruthy();
  });

  it('history rỗng → "Chưa có log"', () => {
    page();
    act(() => FakeES.last!.emit('history', []));
    expect(screen.getByText('Chưa có log')).toBeTruthy();
  });

  it('hiện lịch sử rồi nối log sống, giữ màu ANSI', () => {
    page();
    act(() => FakeES.last!.emit('history', [line(1, 'đầu tiên\n'), line(2, '\x1b[32mxanh\x1b[0m\n')]));
    const out = screen.getByTestId('terminal-output');
    expect(out.textContent).toContain('đầu tiên');
    const green = Array.from(out.querySelectorAll('span')).find((s) => s.textContent === 'xanh');
    expect(green?.getAttribute('data-color')).toBe('green');
    act(() => FakeES.last!.emit('log', line(3, 'sống\n')));
    expect(out.textContent).toMatch(/đầu tiên[\s\S]*xanh[\s\S]*sống/);
    expect(out.textContent).not.toContain('\x1b');
  });

  it('giới hạn số dòng hiển thị (giữ dòng mới nhất)', () => {
    page();
    act(() => FakeES.last!.emit('history', Array.from({ length: 6000 }, (_, i) => line(i + 1, `dòng ${i + 1}\n`))));
    const out = screen.getByTestId('terminal-output');
    expect(out.textContent).toContain('dòng 6000');
    expect(out.textContent).not.toContain('dòng 1\n');
    expect(out.querySelectorAll('[data-line]').length).toBeLessThanOrEqual(5000);
  });

  it('lọc theo task', () => {
    page();
    act(() => FakeES.last!.emit('history', [line(1, 'của t1\n', 'a1', 't1'), line(2, 'của t2\n', 'a2', 't2')]));
    fireEvent.change(screen.getByLabelText('Lọc theo task'), { target: { value: 't2' } });
    const out = screen.getByTestId('terminal-output');
    expect(out.textContent).toContain('của t2');
    expect(out.textContent).not.toContain('của t1');
  });

  it('mất kết nối → banner; nối lại thì ẩn; rời trang đóng luồng', () => {
    const { unmount } = page();
    const es = FakeES.last!;
    act(() => es.emit('history', [line(1, 'x\n')]));
    act(() => es.onerror?.(new Event('error')));
    expect(screen.getByRole('alert').textContent).toMatch(/Mất kết nối/);
    act(() => es.onopen?.(new Event('open')));
    expect(screen.queryByRole('alert')).toBeNull();
    unmount();
    expect(es.closed).toBe(true);
  });
});
