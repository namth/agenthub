// Test văn phòng bản 3 — do trưởng phòng viết.
// Đặc tả: docs/features/virtual-office.md §10, DEC-44; FEAT-11 (nhiều tài khoản cùng loại trên một máy).
import { act, cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { computeLayout, initialState, type Rect, type Snapshot } from './model';
import { createAnimState, syncAnimState } from './renderer';
import Office from '../pages/Office';

const overlap = (a: Rect, b: Rect) => a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
const inside = (a: Rect, o: Rect) => a.x >= o.x && a.y >= o.y && a.x + a.w <= o.x + o.w && a.y + a.h <= o.y + o.h;

const threeClaude: Snapshot['workers'][number] = {
  id: 'mac',
  conn_status: 'ONLINE',
  agents: [
    { id: 'mac/claude', kind: 'CLAUDE', status: 'IDLE' },
    { id: 'mac/claude-2', kind: 'CLAUDE', status: 'IDLE' },
    { id: 'mac/claude-3', kind: 'CLAUDE', status: 'COOLDOWN', cooldown_until: '2026-11-05T12:30:00Z' },
  ],
};
const users3 = [
  { login: 'namth', avatar_url: '', is_owner: true, online: true },
  { login: 'an', avatar_url: '', is_owner: false, online: true },
  { login: 'binh', avatar_url: '', is_owner: false, online: false },
];

describe('bố cục bản 3', () => {
  it('3 tài khoản Claude cùng một máy → 3 bàn, không bàn trống', () => {
    const l = computeLayout([threeClaude], []);
    expect(l.seats.length).toBe(3);
    expect(l.seats.every((s) => s.agentId && s.workerId === 'mac')).toBe(true);
    expect(new Set(l.seats.map((s) => s.agentId)).size).toBe(3);
  });

  it('tường sau: cửa sổ kính ở giữa khu làm việc, logo bên phải cửa sổ', () => {
    const l = computeLayout([threeClaude], []);
    const z = l.zones;
    expect(inside(z.window, z.backWall)).toBe(true);
    const workMid = z.work.x + z.work.w / 2;
    expect(z.window.x).toBeLessThan(workMid);
    expect(z.window.x + z.window.w).toBeGreaterThan(workMid);
    expect(z.logo.x).toBeGreaterThanOrEqual(z.window.x + z.window.w);
    expect(z.logo.x + z.logo.w).toBeLessThanOrEqual(z.work.x + z.work.w);
    expect(overlap(z.logo, z.window)).toBe(false);
    expect(overlap(z.board, z.window)).toBe(false);
  });

  it('phòng nghỉ có 2–4 giường nằm trong phòng, không chồng nhau', () => {
    const l = computeLayout([threeClaude], []);
    const beds = l.zones.beds;
    expect(beds.length).toBeGreaterThanOrEqual(2);
    expect(beds.length).toBeLessThanOrEqual(4);
    for (const b of beds) expect(inside(b, l.zones.breakRoom)).toBe(true);
    for (let i = 0; i < beds.length; i++) for (let j = i + 1; j < beds.length; j++) expect(overlap(beds[i], beds[j])).toBe(false);
  });

  it('khu chỉ huy: bàn owner/partner cách nhau ≥ 8 px, trong khu chỉ huy', () => {
    const l = computeLayout([threeClaude], users3);
    const d = l.userDesks;
    expect(d.length).toBe(3);
    for (let i = 1; i < d.length; i++) expect(d[i].y - (d[i - 1].y + d[i - 1].h)).toBeGreaterThanOrEqual(8);
    for (const x of d) expect(inside(x, l.zones.command)).toBe(true);
  });

  it('agent COOLDOWN nằm trên giường; trưởng phòng ngồi quay mặt ra; nhân viên quay lưng', () => {
    const snap: Snapshot = { lead: { state: 'HELD', agent_id: 'mac/claude', epoch: 1 }, workers: [threeClaude], users: [], recent: [] };
    const l = computeLayout(snap.workers, []);
    const st = initialState(snap);
    const anim = createAnimState();
    syncAnimState(anim, st, l, true);
    const sleeper = anim.chars['mac/claude-3'];
    expect(sleeper.pose).toBe('lie');
    const bed = l.zones.beds.find((b) => sleeper.x >= b.x && sleeper.x <= b.x + b.w && sleeper.y >= b.y && sleeper.y <= b.y + b.h);
    expect(bed).toBeTruthy();
    expect(anim.chars['mac/claude'].pose).toBe('sit-front');
    expect(anim.chars['mac/claude'].facing).toBe('front');
    expect(anim.chars['mac/claude-2'].pose).toBe('sit-back');
    expect(anim.chars['mac/claude-2'].facing).toBe('back');
  });
});

// ---------- màn hình ----------

class FakeES {
  static last: FakeES | null = null;
  url: string;
  onerror: ((e: unknown) => void) | null = null;
  onopen: ((e: unknown) => void) | null = null;
  listeners: Record<string, ((e: MessageEvent) => void)[]> = {};
  constructor(url: string) { this.url = url; FakeES.last = this; }
  addEventListener(type: string, fn: (e: MessageEvent) => void) { (this.listeners[type] ??= []).push(fn); }
  removeEventListener() {}
  close() {}
  emit(type: string, data: unknown) {
    for (const fn of this.listeners[type] ?? []) fn(new MessageEvent(type, { data: JSON.stringify(data) }));
  }
}

beforeEach(() => { FakeES.last = null; vi.stubGlobal('EventSource', FakeES); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe('SCR-08 bản 3', () => {
  it('canvas vẽ ở độ phân giải thật (≥ 2 × logic) để chữ nét', () => {
    render(<MemoryRouter><Office /></MemoryRouter>);
    act(() => FakeES.last!.emit('snapshot', { lead: { state: 'VACANT', agent_id: null, epoch: 0 }, workers: [threeClaude], users: users3, recent: [] }));
    const cv = screen.getByTestId('office-canvas') as HTMLCanvasElement;
    const l = computeLayout([threeClaude], users3);
    expect(cv.width).toBeGreaterThanOrEqual(l.width * 2);
    expect(cv.width % l.width).toBe(0);
    expect(cv.height).toBe((cv.width / l.width) * l.height);
    expect(screen.getAllByTestId(/^seat-\d+$/).length).toBe(3);
    expect(screen.getByTestId('char-mac/claude-3').getAttribute('data-place')).toBe('break');
  });
});
