// Test nghiệm thu TASK-035 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/virtual-office.md bản 2 §4 (bố cục), §7 BR-43 → BR-46; TC-33 (sửa), TC-34, TC-71 → TC-73.
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { applyEvent, computeLayout, initialState, type Rect, type Snapshot } from './model';
import Office from '../pages/Office';

type W = Snapshot['workers'][number];
const mk = (id: string, kinds: string[] = ['CLAUDE', 'ANTIGRAVITY']): W => ({
  id,
  conn_status: 'ONLINE',
  agents: kinds.map((k, i) => ({ id: `${id}/${k === 'CLAUDE' ? 'claude' : k === 'ANTIGRAVITY' ? 'antigravity' : `x${i}`}`, kind: k, status: 'IDLE' })),
});
const names = (n: number) => Array.from({ length: n }, (_, i) => mk(`m${String(i).padStart(2, '0')}`));
const users = (n: number) =>
  Array.from({ length: n }, (_, i) => ({ login: i === 0 ? 'namth' : `p${i}`, avatar_url: '', is_owner: i === 0, online: i < 2 }));

const overlap = (a: Rect, b: Rect) => a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
const inside = (a: Rect, outer: { x?: number; y?: number; w?: number; h?: number; width?: number; height?: number }) => {
  const ox = outer.x ?? 0, oy = outer.y ?? 0, ow = outer.w ?? outer.width!, oh = outer.h ?? outer.height!;
  return a.x >= ox && a.y >= oy && a.x + a.w <= ox + ow && a.y + a.h <= oy + oh;
};
const seatOf = (l: ReturnType<typeof computeLayout>, agentId: string) => l.seats.find((s) => s.agentId === agentId)!;

// ---------- bố cục khu làm việc (TC-33 sửa, BR-43) ----------

describe('computeLayout bản 2', () => {
  it('0 máy: đủ 8 bàn trống, 2 cụm × 2 hàng × 2 bàn', () => {
    const l = computeLayout([], []);
    expect(l.seats.length).toBe(8);
    expect(l.seats.every((s) => s.agentId === null && s.workerId === null)).toBe(true);
    expect(l.seats.filter((s) => s.cluster === 'left').length).toBe(4);
    expect(l.seats.filter((s) => s.cluster === 'right').length).toBe(4);
    expect(new Set(l.seats.map((s) => `${s.cluster}-${s.row}-${s.col}`)).size).toBe(8);
  });

  it('thứ tự trái-1 → phải-1 → trái-2 → phải-2, máy sắp theo tên, claude trước agy', () => {
    const l = computeLayout([mk('dd'), mk('Bb'), mk('cc'), mk('aa')], []);
    const at = (id: string) => { const s = seatOf(l, id); return `${s.cluster}-${s.row}-${s.col}`; };
    expect(at('aa/claude')).toBe('left-0-0');
    expect(at('aa/antigravity')).toBe('left-0-1');
    expect(at('Bb/claude')).toBe('right-0-0');
    expect(at('cc/claude')).toBe('left-1-0');
    expect(at('dd/antigravity')).toBe('right-1-1');
    expect(l.seats.length).toBe(8);
    expect(l.machineLabels.map((m) => m.workerId).sort()).toEqual(['Bb', 'aa', 'cc', 'dd'].sort());
  });

  it('1 máy 1 agent: bàn thứ hai của hàng để trống', () => {
    const l = computeLayout([mk('solo', ['ANTIGRAVITY'])], []);
    const s = seatOf(l, 'solo/antigravity');
    expect(`${s.cluster}-${s.row}-${s.col}`).toBe('left-0-0');
    const mate = l.seats.find((x) => x.cluster === 'left' && x.row === 0 && x.col === 1)!;
    expect(mate.agentId).toBeNull();
  });

  it('5 máy: cụm trái 3 hàng, phải 2 hàng (10 bàn); 9 máy: 5 + 4 hàng (18 bàn); canvas cao dần', () => {
    const l5 = computeLayout(names(5), []);
    expect(l5.seats.length).toBe(10);
    expect(Math.max(...l5.seats.filter((s) => s.cluster === 'left').map((s) => s.row))).toBe(2);
    expect(Math.max(...l5.seats.filter((s) => s.cluster === 'right').map((s) => s.row))).toBe(1);
    const l9 = computeLayout(names(9), []);
    expect(l9.seats.length).toBe(18);
    expect(seatOf(l9, 'm08/claude').cluster).toBe('left');
    expect(seatOf(l9, 'm08/claude').row).toBe(4);
    const l4 = computeLayout(names(4), []);
    expect(l9.height).toBeGreaterThan(l4.height);
    expect(l4.height).toBe(computeLayout([], []).height);
  });

  it('máy 3 agent chiếm 2 hàng liền trong cùng cụm', () => {
    const l = computeLayout([mk('big', ['CLAUDE', 'ANTIGRAVITY', 'OTHER'])], []);
    const ss = l.seats.filter((s) => s.workerId === 'big');
    expect(ss.length).toBe(3);
    expect(new Set(ss.map((s) => s.cluster)).size).toBe(1);
    expect(ss.map((s) => s.row).sort()).toEqual([0, 0, 1]);
  });

  for (const n of [0, 3, 9]) {
    it(`${n} máy: không chồng lấn, lối đi giữa, bàn trưởng phòng ở trên, phòng nghỉ + sảnh ở đáy, khu chỉ huy bên phải`, () => {
      const l = computeLayout(names(n), users(3));
      const z = l.zones;
      for (const s of l.seats) {
        expect(inside(s, z.work)).toBe(true);
        expect(overlap(s, z.aisle)).toBe(false);
        if (s.cluster === 'left') expect(s.x + s.w).toBeLessThanOrEqual(z.aisle.x);
        else expect(s.x).toBeGreaterThanOrEqual(z.aisle.x + z.aisle.w);
        expect(z.leadDesk.y + z.leadDesk.h).toBeLessThanOrEqual(s.y);
        expect(s.y + s.h).toBeLessThanOrEqual(z.breakRoom.y);
        expect(s.y + s.h).toBeLessThanOrEqual(z.lobby.y);
      }
      for (let i = 0; i < l.seats.length; i++)
        for (let j = i + 1; j < l.seats.length; j++) expect(overlap(l.seats[i], l.seats[j])).toBe(false);
      expect(inside(z.logo, z.backWall)).toBe(true);
      expect(z.logo.y + z.logo.h).toBeLessThanOrEqual(z.leadDesk.y);
      expect(z.command.x).toBeGreaterThanOrEqual(z.work.x + z.work.w);
      expect(overlap(z.commandDoor, z.command) || z.commandDoor.x <= z.command.x).toBe(true);
      expect(overlap(z.breakRoom, z.lobby)).toBe(false);
      expect(inside(z.lobbyRug, z.lobby)).toBe(true);
      for (const r of [z.work, z.command, z.breakRoom, z.lobby, z.backWall]) expect(inside(r, l)).toBe(true);
    });
  }
});

// ---------- khu chỉ huy (TC-71, BR-44) ----------

describe('khu chỉ huy', () => {
  it('mỗi tài khoản một bàn, xếp dọc theo thứ tự server, trong khu chỉ huy', () => {
    const l = computeLayout(names(2), users(4));
    expect(l.userDesks.map((d) => d.login)).toEqual(['namth', 'p1', 'p2', 'p3']);
    const xs = new Set(l.userDesks.map((d) => d.x));
    expect(xs.size).toBe(1);
    for (let i = 1; i < l.userDesks.length; i++) expect(l.userDesks[i].y).toBeGreaterThan(l.userDesks[i - 1].y + l.userDesks[i - 1].h - 1);
    for (const d of l.userDesks) expect(inside(d, l.zones.command)).toBe(true);
    expect(l.userDesks[0].isOwner).toBe(true);
    expect(l.userDesks[3].online).toBe(false);
  });

  it('12 người: canvas cao thêm, bàn cuối không chồng sảnh', () => {
    const small = computeLayout(names(2), users(2));
    const big = computeLayout(names(2), users(12));
    expect(big.height).toBeGreaterThan(small.height);
    const lastD = big.userDesks[big.userDesks.length - 1];
    expect(lastD.y + lastD.h).toBeLessThanOrEqual(big.zones.lobby.y);
    expect(lastD.y + lastD.h).toBeLessThanOrEqual(big.zones.breakRoom.y);
  });
});

// ---------- trạng thái: NEEDS_AUTH, presence (TC-72, BR-45) ----------

const snap2: Snapshot = {
  lead: { state: 'HELD', agent_id: 'mac/claude', epoch: 3 },
  workers: [
    { id: 'mac', conn_status: 'ONLINE', gh_auth: 'OK', agents: [
      { id: 'mac/claude', kind: 'CLAUDE', status: 'IDLE', auth_status: 'OK' },
      { id: 'mac/antigravity', kind: 'ANTIGRAVITY', status: 'IDLE', auth_status: 'NEEDS_AUTH' },
    ] },
    { id: 'box', conn_status: 'ONLINE', gh_auth: 'NEEDS_AUTH', agents: [{ id: 'box/claude', kind: 'CLAUDE', status: 'IDLE', auth_status: 'UNKNOWN' }] },
  ],
  users: [
    { login: 'namth', avatar_url: '', is_owner: true, online: true },
    { login: 'an', avatar_url: '', is_owner: false, online: false },
  ],
  recent: [],
};

describe('initialState / applyEvent bản 2', () => {
  it('NEEDS_AUTH → đứng cạnh bàn (place auth); gh máy NEEDS_AUTH; người dùng online', () => {
    const s = initialState(snap2);
    expect(s.chars['mac/antigravity'].place).toBe('auth');
    expect(s.chars['box/claude'].place).toBe('desk');
    expect(s.ghNeeds.has('box')).toBe(true);
    expect(s.users['namth'].online).toBe(true);
    expect(s.users['an'].online).toBe(false);
  });

  it('sự kiện auth và presence', () => {
    let s = initialState(snap2);
    s = applyEvent(s, { id: 1, ts: '', who: 'mac/antigravity', type: 'auth', text: 'OK' });
    expect(s.chars['mac/antigravity'].place).toBe('desk');
    s = applyEvent(s, { id: 2, ts: '', who: 'box/claude', type: 'auth', text: 'NEEDS_AUTH' });
    expect(s.chars['box/claude'].place).toBe('auth');
    s = applyEvent(s, { id: 3, ts: '', who: 'box', type: 'auth', text: 'OK' });
    expect(s.ghNeeds.has('box')).toBe(false);
    s = applyEvent(s, { id: 4, ts: '', who: 'user:an', type: 'presence', text: 'online' });
    expect(s.users['an'].online).toBe(true);
    s = applyEvent(s, { id: 5, ts: '', who: 'user:namth', type: 'presence', text: 'offline' });
    expect(s.users['namth'].online).toBe(false);
  });

  it('handover tới Antigravity (DEC-29): lên ghế trưởng phòng', () => {
    let s = initialState(snap2);
    s = applyEvent(s, { id: 1, ts: '', who: 'mac/claude', type: 'handover', to: 'box/claude' });
    expect(s.chars['box/claude'].place).toBe('lead-seat');
    s = applyEvent(s, { id: 2, ts: '', who: 'box/claude', type: 'handover', to: 'mac/antigravity' });
    expect(s.chars['mac/antigravity'].place).toBe('lead-seat');
  });
});

// ---------- màn hình (TC-34, TC-72, TC-73) ----------

class FakeES {
  static last: FakeES | null = null;
  url: string;
  onerror: ((e: unknown) => void) | null = null;
  onopen: ((e: unknown) => void) | null = null;
  listeners: Record<string, ((e: MessageEvent) => void)[]> = {};
  closed = false;
  constructor(url: string) { this.url = url; FakeES.last = this; }
  addEventListener(type: string, fn: (e: MessageEvent) => void) { (this.listeners[type] ??= []).push(fn); }
  removeEventListener() {}
  close() { this.closed = true; }
  emit(type: string, data: unknown, id?: string) {
    for (const fn of this.listeners[type] ?? []) fn(new MessageEvent(type, { data: JSON.stringify(data), lastEventId: id }));
  }
}

beforeEach(() => { FakeES.last = null; vi.stubGlobal('EventSource', FakeES); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const page = () => render(<MemoryRouter><Office /></MemoryRouter>);

describe('SCR-08 bản 2', () => {
  it('vẽ đủ 8 ghế (ghế trống đánh dấu), khu chỉ huy có bàn theo người, 🔑 cho agent NEEDS_AUTH', () => {
    page();
    act(() => FakeES.last!.emit('snapshot', snap2));
    const seats = screen.getAllByTestId(/^seat-\d+$/);
    expect(seats.length).toBe(8);
    expect(seats.filter((s) => s.getAttribute('data-empty') === 'true').length).toBe(5);
    expect(screen.getByTestId('user-namth').getAttribute('data-online')).toBe('true');
    expect(screen.getByTestId('user-an').getAttribute('data-online')).toBe('false');
    expect(screen.getByTestId('user-an').textContent).toContain('an');
    const agy = screen.getByTestId('char-mac/antigravity');
    expect(agy.getAttribute('data-place')).toBe('auth');
    expect(agy.textContent).toContain('🔑');
    expect(screen.getByTestId('machine-label-box').textContent).toContain('🔑');
    act(() => FakeES.last!.emit('office', { id: 7, ts: '', who: 'user:an', type: 'presence', text: 'online' }, '7'));
    expect(screen.getByTestId('user-an').getAttribute('data-online')).toBe('true');
  });

  it('logo INOVA trên tường + icon ở cửa kính và thảm sảnh; lỗi tải → chữ INOVA', () => {
    page();
    act(() => FakeES.last!.emit('snapshot', snap2));
    const logo = screen.getByAltText('INOVA') as HTMLImageElement;
    expect(logo.getAttribute('src')).toBe('/brand/inova-logo.png');
    const icons = screen.getAllByTestId(/^inova-icon-/) as HTMLImageElement[];
    expect(icons.length).toBeGreaterThanOrEqual(2);
    for (const i of icons) expect(i.getAttribute('src')).toBe('/brand/inova-icon.png');
    expect(screen.queryByTestId('logo-fallback')).toBeNull();
    fireEvent.error(logo);
    expect(screen.getByTestId('logo-fallback').textContent).toBe('INOVA');
  });

  it('thanh dưới đếm Cần đăng nhập và Online', () => {
    page();
    act(() => FakeES.last!.emit('snapshot', snap2));
    const bar = screen.getByTestId('office-status');
    expect(bar.textContent).toMatch(/Cần đăng nhập 1/);
    expect(bar.textContent).toMatch(/Online 1\/2/);
  });
});
