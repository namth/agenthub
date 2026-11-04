// Test nghiệm thu TASK-012 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/virtual-office.md §1, §3, §4 (DEC-07, DEC-08; sửa theo bản 2 — TASK-035); luồng SSE theo
// internal/office/office_test.go.
import { act, cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { initialState, applyEvent, type Snapshot } from './model';
import Office from '../pages/Office';

const snap: Snapshot = {
  lead: { state: 'HELD', agent_id: 'mac/claude', epoch: 7 },
  workers: [
    { id: 'mac', conn_status: 'ONLINE', agents: [{ id: 'mac/claude', kind: 'CLAUDE', status: 'BUSY' }, { id: 'mac/antigravity', kind: 'ANTIGRAVITY', status: 'IDLE' }] },
    { id: 'lap', conn_status: 'ONLINE', agents: [{ id: 'lap/claude', kind: 'CLAUDE', status: 'COOLDOWN', cooldown_until: '2026-11-04T14:40:00Z' }] },
    { id: 'srv', conn_status: 'OFFLINE', agents: [{ id: 'srv/antigravity', kind: 'ANTIGRAVITY', status: 'IDLE' }] },
  ],
  recent: [],
};

// ---------- bố cục: bản 2 thay thế, xem office_v2.test.tsx (TC-33 sửa, DEC-25) ----------

// ---------- trạng thái (TC-34, TC-35) ----------

describe('initialState / applyEvent', () => {
  it('dựng từ snapshot: lead ngồi ghế trưởng phòng, COOLDOWN ở phòng nghỉ, máy OFFLINE tắt đèn', () => {
    const s = initialState(snap);
    expect(s.lead).toBe('mac/claude');
    expect(s.chars['mac/claude'].place).toBe('lead-seat');
    expect(s.chars['mac/antigravity'].place).toBe('desk');
    expect(s.chars['lap/claude'].place).toBe('break');
    expect(s.chars['lap/claude'].note).toMatch(/\d{2}:\d{2}/);
    expect(s.chars['srv/antigravity'].place).toBe('offline');
    expect(s.dark.has('srv')).toBe(true);
  });

  it('handover: lead cũ về bàn, lead mới lên ghế trưởng phòng', () => {
    let s = initialState(snap);
    s = applyEvent(s, { id: 1, ts: '', who: 'mac/claude', type: 'handover', to: 'mac/antigravity', text: 'epoch 8' });
    expect(s.lead).toBe('mac/antigravity');
    expect(s.chars['mac/antigravity'].place).toBe('lead-seat');
    expect(s.chars['mac/claude'].place).toBe('desk');
  });

  it('cooldown → phòng nghỉ, idle → về bàn; vacant → ghế trống', () => {
    let s = initialState(snap);
    s = applyEvent(s, { id: 1, ts: '', who: 'mac/antigravity', type: 'cooldown', text: 'đến 21:40' });
    expect(s.chars['mac/antigravity'].place).toBe('break');
    expect(s.chars['mac/antigravity'].note).toContain('21:40');
    s = applyEvent(s, { id: 2, ts: '', who: 'mac/antigravity', type: 'idle' });
    expect(s.chars['mac/antigravity'].place).toBe('desk');
    s = applyEvent(s, { id: 3, ts: '', who: 'system', type: 'vacant', text: 'dự kiến 21:40' });
    expect(s.lead).toBeNull();
    expect(s.vacantNote).toContain('21:40');
    expect(Object.values(s.chars).some((c) => c.place === 'lead-seat')).toBe(false);
  });

  it('assign/submit gắn và gỡ task; offline/online đổi cả dãy', () => {
    let s = initialState(snap);
    s = applyEvent(s, { id: 1, ts: '', who: 'mac/claude', type: 'assign', to: 'mac/antigravity', task: 't102' });
    expect(s.chars['mac/antigravity'].task).toBe('t102');
    s = applyEvent(s, { id: 2, ts: '', who: 'mac/antigravity', type: 'submit' });
    expect(s.chars['mac/antigravity'].task).toBeUndefined();
    s = applyEvent(s, { id: 3, ts: '', who: 'mac', type: 'offline' });
    expect(s.dark.has('mac')).toBe(true);
    expect(s.chars['mac/antigravity'].place).toBe('offline');
    s = applyEvent(s, { id: 4, ts: '', who: 'srv', type: 'online' });
    expect(s.dark.has('srv')).toBe(false);
    expect(s.chars['srv/antigravity'].place).toBe('desk');
  });

  it('applyEvent không đổi state cũ (bất biến) và bỏ qua sự kiện lạ', () => {
    const s0 = initialState(snap);
    const s1 = applyEvent(s0, { id: 1, ts: '', who: 'x', type: 'la-gi-do' });
    expect(s1.chars).toEqual(s0.chars);
    applyEvent(s0, { id: 2, ts: '', who: 'mac/antigravity', type: 'cooldown', text: 'đến 21:40' });
    expect(s0.chars['mac/antigravity'].place).toBe('desk');
  });
});

// ---------- màn hình (4 trạng thái) ----------

class FakeES {
  static last: FakeES | null = null;
  url: string;
  onerror: ((e: unknown) => void) | null = null;
  onopen: ((e: unknown) => void) | null = null;
  listeners: Record<string, ((e: MessageEvent) => void)[]> = {};
  closed = false;
  constructor(url: string) {
    this.url = url;
    FakeES.last = this;
  }
  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    (this.listeners[type] ??= []).push(fn);
  }
  removeEventListener() {}
  close() {
    this.closed = true;
  }
  emit(type: string, data: unknown, id?: string) {
    for (const fn of this.listeners[type] ?? []) fn(new MessageEvent(type, { data: JSON.stringify(data), lastEventId: id }));
  }
}

beforeEach(() => {
  FakeES.last = null;
  vi.stubGlobal('EventSource', FakeES);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const page = () => render(<MemoryRouter><Office /></MemoryRouter>);

describe('SCR-08 Office', () => {
  it('mở luồng /api/office/stream (DEC-27); đang tải → aria-busy', () => {
    const { container } = page();
    expect(FakeES.last?.url).toBe('/api/office/stream');
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
  });

  it('snapshot → canvas + nhân vật theo vị trí; sự kiện sống cập nhật', () => {
    page();
    act(() => FakeES.last!.emit('snapshot', snap));
    expect(screen.getByTestId('office-canvas')).toBeTruthy();
    expect(screen.getByTestId('char-mac/claude').getAttribute('data-place')).toBe('lead-seat');
    expect(screen.getByTestId('char-lap/claude').getAttribute('data-place')).toBe('break');
    act(() => FakeES.last!.emit('office', { id: 9, ts: '', who: 'mac/claude', type: 'handover', to: 'lap/claude' }, '9'));
    expect(screen.getByTestId('char-lap/claude').getAttribute('data-place')).toBe('lead-seat');
  });

  it('rỗng → thông điệp và link thêm máy (SCR-11, bản 2)', () => {
    page();
    act(() => FakeES.last!.emit('snapshot', { lead: { state: 'VACANT', agent_id: null, epoch: 0 }, workers: [], recent: [] }));
    expect(screen.getByText('Chưa có máy nào kết nối')).toBeTruthy();
    expect(screen.getByRole('link', { name: /Thêm máy/ }).getAttribute('href')).toBe('/my/machines');
  });

  it('mất kết nối → banner, giữ cảnh cuối', () => {
    page();
    act(() => FakeES.last!.emit('snapshot', snap));
    act(() => FakeES.last!.onerror?.(new Event('error')));
    expect(screen.getByRole('alert').textContent).toMatch(/Mất kết nối/);
    expect(screen.getByTestId('char-mac/claude')).toBeTruthy();
    act(() => FakeES.last!.onopen?.(new Event('open')));
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('rời trang thì đóng luồng', () => {
    const { unmount } = page();
    const es = FakeES.last!;
    unmount();
    expect(es.closed).toBe(true);
  });
});
