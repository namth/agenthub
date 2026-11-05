import type { computeLayout, OfficeState, Place } from './model';

type Layout = ReturnType<typeof computeLayout>;

export type CharAnim = {
  x: number;
  y: number;
  pts: { x: number; y: number }[];
  targetX: number;
  targetY: number;
  walking: boolean;
  facing: 'front' | 'back';
  pose: Pose;
  restPose: Pose;
};

export type Pose = 'sit-back' | 'sit-front' | 'lie' | 'stand';

const facingOf = (p: Pose): 'front' | 'back' => (p === 'sit-back' ? 'back' : 'front');

export type AnimState = {
  chars: Record<string, CharAnim>;
  lastTime: number;
  lastFrameTime: number;
};

export function createAnimState(): AnimState {
  return {
    chars: {},
    lastTime: Date.now(),
    lastFrameTime: 0,
  };
}

function getCharTarget(
  id: string,
  place: Place,
  _workerId: string,
  layout: Layout,
  breakIdx: number
): { x: number; y: number; pose: Pose } {
  if (place === 'lead-seat') {
    const z = layout.zones.leadDesk;
    return { x: z.x + Math.floor(z.w / 2), y: z.y + 6, pose: 'sit-front' };
  }
  if (place === 'break') {
    const beds = layout.zones.beds;
    if (breakIdx < beds.length) {
      const b = beds[breakIdx];
      return { x: b.x + 14, y: b.y + 12, pose: 'lie' };
    }
    const so = layout.zones.sofa;
    const k = (breakIdx - beds.length) % 3;
    return { x: so.x + 12 + k * 16, y: so.y + 14, pose: 'sit-front' };
  }
  const seat = layout.seats.find((s) => s.agentId === id);
  if (place === 'auth' && seat) {
    const offsetX = seat.col === 0 ? -9 : seat.w + 9;
    return { x: seat.x + offsetX, y: seat.y + 24, pose: 'stand' };
  }
  // 'desk' hoặc 'offline': ngồi ghế, quay lưng về người xem, nhìn vào màn hình.
  if (seat) {
    return { x: seat.x + Math.floor(seat.w / 2), y: seat.y + 27, pose: 'sit-back' };
  }
  return { x: 50, y: 100, pose: 'stand' };
}

function routeTo(
  from: { x: number; y: number },
  to: { x: number; y: number },
  layout: Layout
): { x: number; y: number }[] {
  const aisleCenter = layout.zones.aisle.x + Math.floor(layout.zones.aisle.w / 2);
  const doorX = layout.zones.commandDoor.x;
  const doorY = layout.zones.commandDoor.y + Math.floor(layout.zones.commandDoor.h / 2);

  const fromInCommand = from.x >= layout.zones.command.x;
  const toInCommand = to.x >= layout.zones.command.x;

  const pts: { x: number; y: number }[] = [];

  if (fromInCommand && toInCommand) {
    // Both in command area
    pts.push({ x: to.x, y: to.y });
    return pts;
  }

  if (fromInCommand && !toInCommand) {
    // Leave command area through door
    pts.push({ x: doorX + 16, y: from.y });
    pts.push({ x: doorX - 8, y: doorY });
    pts.push({ x: aisleCenter, y: doorY });
    pts.push({ x: aisleCenter, y: to.y });
    pts.push({ x: to.x, y: to.y });
    return pts;
  }

  if (!fromInCommand && toInCommand) {
    // Enter command area through door
    pts.push({ x: aisleCenter, y: from.y });
    pts.push({ x: aisleCenter, y: doorY });
    pts.push({ x: doorX - 8, y: doorY });
    pts.push({ x: doorX + 16, y: doorY });
    pts.push({ x: to.x, y: to.y });
    return pts;
  }

  // Within work / break / lobby area
  pts.push({ x: aisleCenter, y: from.y });
  pts.push({ x: aisleCenter, y: to.y });
  pts.push({ x: to.x, y: to.y });

  return pts;
}

export function syncAnimState(
  anim: AnimState,
  state: OfficeState,
  layout: Layout,
  snapOnly: boolean
) {
  let breakCount = 0;
  for (const [id, c] of Object.entries(state.chars)) {
    const bIdx = c.place === 'break' ? breakCount++ : 0;
    const target = getCharTarget(id, c.place, c.worker, layout, bIdx);

    if (!anim.chars[id] || snapOnly) {
      anim.chars[id] = {
        x: target.x,
        y: target.y,
        pts: [],
        targetX: target.x,
        targetY: target.y,
        walking: false,
        facing: facingOf(target.pose),
        pose: target.pose,
        restPose: target.pose,
      };
    } else {
      const cur = anim.chars[id];
      cur.restPose = target.pose;
      if (Math.hypot(cur.targetX - target.x, cur.targetY - target.y) > 2) {
        cur.targetX = target.x;
        cur.targetY = target.y;
        cur.pose = 'stand';
        cur.pts = routeTo({ x: cur.x, y: cur.y }, target, layout);
      } else if (!cur.walking) {
        cur.pose = target.pose;
        cur.facing = facingOf(target.pose);
      }
    }
  }
}

export function stepAnim(anim: AnimState, dt: number): boolean {
  let moving = false;
  const speed = 64; // px/s

  for (const c of Object.values(anim.chars)) {
    if (c.pts && c.pts.length > 0) {
      moving = true;
      c.walking = true;
      const nextPt = c.pts[0];
      const dx = nextPt.x - c.x;
      const dy = nextPt.y - c.y;
      const dist = Math.hypot(dx, dy);

      if (dist <= speed * dt) {
        c.x = nextPt.x;
        c.y = nextPt.y;
        c.pts.shift();
      } else {
        c.x += (dx / dist) * (speed * dt);
        c.y += (dy / dist) * (speed * dt);
      }
      c.facing = dy < -0.1 ? 'back' : 'front';
    } else {
      const dx = c.targetX - c.x;
      const dy = c.targetY - c.y;
      const dist = Math.hypot(dx, dy);

      if (dist > 1) {
        moving = true;
        c.walking = true;
        const move = Math.min(dist, speed * dt);
        c.x += (dx / dist) * move;
        c.y += (dy / dist) * move;
        c.facing = dy < -0.1 ? 'back' : 'front';
      } else {
        c.x = c.targetX;
        c.y = c.targetY;
        if (c.walking) {
          c.pose = c.restPose;
          c.facing = facingOf(c.restPose);
        }
        c.walking = false;
      }
    }
  }

  return moving;
}

export function drawPlant(ctx: CanvasRenderingContext2D, x: number, y: number) {
  const R = (c: string, rx: number, ry: number, rw: number, rh: number) => {
    ctx.fillStyle = c;
    ctx.fillRect(Math.round(rx), Math.round(ry), Math.round(rw), Math.round(rh));
  };
  R('#3f8f4a', x - 5, y - 14, 10, 8);
  R('#4caf5a', x - 3, y - 17, 6, 5);
  R('#2f6f39', x - 1, y - 12, 2, 5);
  R('#b5651d', x - 4, y - 6, 8, 6);
  R('#9c5418', x - 4, y - 6, 8, 1);
}

function drawClock(ctx: CanvasRenderingContext2D, cx: number, cy: number) {
  const R = (c: string, rx: number, ry: number, rw: number, rh: number) => {
    ctx.fillStyle = c;
    ctx.fillRect(Math.round(rx), Math.round(ry), Math.round(rw), Math.round(rh));
  };
  for (let dy = -6; dy <= 6; dy++) {
    const w = Math.floor(Math.sqrt(40 - dy * dy));
    R('#2a2d40', cx - w - 1, cy + dy, 2 * w + 3, 1);
  }
  for (let dy = -5; dy <= 5; dy++) {
    const w = Math.floor(Math.sqrt(30 - dy * dy));
    R('#f4f4ef', cx - w, cy + dy, 2 * w + 1, 1);
  }

  const d = new Date();
  const hAng =
    (((d.getHours() % 12) + d.getMinutes() / 60) / 12) * 2 * Math.PI - Math.PI / 2;
  const mAng = (d.getMinutes() / 60) * 2 * Math.PI - Math.PI / 2;

  for (let k = 1; k <= 3; k++) {
    R('#2a2d40', Math.round(cx + Math.cos(hAng) * k), Math.round(cy + Math.sin(hAng) * k), 1, 1);
  }
  for (let k = 1; k <= 5; k++) {
    R('#e5484d', Math.round(cx + Math.cos(mAng) * k), Math.round(cy + Math.sin(mAng) * k), 1, 1);
  }
  R('#2a2d40', cx, cy, 1, 1);
}

type Fill = (c: string, x: number, y: number, w: number, h: number) => void;

function filler(ctx: CanvasRenderingContext2D): Fill {
  return (c, x, y, w, h) => {
    ctx.fillStyle = c;
    ctx.fillRect(Math.round(x), Math.round(y), Math.round(w), Math.round(h));
  };
}

function drawWindow(R: Fill, w: { x: number; y: number; w: number; h: number }) {
  // Khung + kính + song cửa + bậu cửa (DEC-44: giữa tường sau là cửa sổ kính).
  R('#cfd8e3', w.x - 2, w.y - 2, w.w + 4, w.h + 4);
  const sky = ['#8ccaf2', '#9dd3f5', '#b0dcf7', '#c4e6f9', '#d6eefb'];
  const band = Math.ceil(w.h / sky.length);
  sky.forEach((c, i) => R(c, w.x, w.y + i * band, w.w, Math.min(band, w.h - i * band)));
  // vài đám mây và toà nhà xa
  R('#ffffff', w.x + 14, w.y + 8, 14, 3);
  R('#ffffff', w.x + 18, w.y + 6, 7, 2);
  R('#ffffff', w.x + 78, w.y + 11, 16, 3);
  R('#a9bfd4', w.x + 6, w.y + w.h - 10, 10, 10);
  R('#9fb3c9', w.x + 20, w.y + w.h - 15, 8, 15);
  R('#b4c7da', w.x + 96, w.y + w.h - 12, 12, 12);
  // ánh phản chiếu chéo
  for (let i = 0; i < 10; i++) R('rgba(255,255,255,0.35)', w.x + 40 + i, w.y + 4 + i * 3, 2, 3);
  // song cửa
  const cols = 3;
  for (let i = 1; i < cols; i++) R('#cfd8e3', w.x + Math.round((w.w / cols) * i) - 1, w.y, 2, w.h);
  R('#cfd8e3', w.x, w.y + Math.round(w.h / 2) - 1, w.w, 2);
  // bậu
  R('#e7edf3', w.x - 4, w.y + w.h + 1, w.w + 8, 3);
  R('#aab6c4', w.x - 4, w.y + w.h + 4, w.w + 8, 1);
}

function drawStaffDesk(R: Fill, s: { x: number; y: number; w: number; h: number }) {
  // Bàn gọn + màn hình phía trên + ghế phía dưới; nhân viên ngồi quay lưng về người xem.
  const cx = s.x + Math.floor(s.w / 2);
  // ghế (bánh xe + đệm), vẽ trước để nhân vật che lên
  R('#23252f', cx - 6, s.y + 28, 12, 1);
  R('#3a3d4d', cx - 1, s.y + 24, 2, 4);
  R('#2b2e3b', cx - 7, s.y + 20, 14, 5);
  R('#3d4155', cx - 7, s.y + 20, 14, 1);
  // bàn
  R('#8d5c33', s.x, s.y + 2, s.w, 12);
  R('#b07a4c', s.x, s.y, s.w, 10);
  R('#c99260', s.x, s.y, s.w, 1);
  R('#6d4426', s.x + 2, s.y + 14, 2, 4);
  R('#6d4426', s.x + s.w - 4, s.y + 14, 2, 4);
  // màn hình (khung) + chân
  R('#2a2d38', cx - 9, s.y - 10, 18, 11);
  R('#555a66', cx - 1, s.y + 1, 2, 2);
  R('#444855', cx - 4, s.y + 3, 8, 1);
  // bàn phím + chuột
  R('#e3e3e3', cx - 6, s.y + 6, 12, 2);
  R('#d0d0d0', cx + 9, s.y + 6, 2, 2);
}

function drawExecChair(R: Fill, cx: number, top: number) {
  R('#1f2230', cx - 8, top, 16, 12);
  R('#2f3346', cx - 7, top + 1, 14, 10);
  R('#3d4258', cx - 7, top + 1, 14, 1);
}

function drawExecDeskFront(R: Fill, d: { x: number; y: number; w: number; h: number }, star: boolean) {
  // Mặt bàn + vách trước; màn hình quay lưng về người xem (người ngồi sau bàn quay mặt ra).
  const mx = d.x + d.w - 20;
  R('#2a2d38', mx, d.y - 8, 14, 9);
  R('#3a3e4b', mx + 1, d.y - 7, 12, 7);
  R('#555a66', mx + 6, d.y + 1, 2, 2);
  R('#b07a4c', d.x, d.y, d.w, 5);
  R('#c99260', d.x, d.y, d.w, 1);
  R('#8d5c33', d.x, d.y + 5, d.w, d.h - 5);
  R('#7a4e2a', d.x + 2, d.y + 7, d.w - 4, d.h - 9);
  if (star) {
    R('#f5c542', d.x + Math.floor(d.w / 2) - 3, d.y + 9, 6, 6);
    R('#fff1b8', d.x + Math.floor(d.w / 2) - 1, d.y + 11, 2, 2);
  } else {
    R('#c8b38a', d.x + Math.floor(d.w / 2) - 8, d.y + 9, 16, 4);
  }
}

function drawBed(R: Fill, b: { x: number; y: number; w: number; h: number }) {
  R('#6e5440', b.x, b.y, b.w, b.h);
  R('#8a6d55', b.x, b.y, 3, b.h);
  R('#e8e4dc', b.x + 3, b.y + 2, b.w - 5, b.h - 4);
  R('#ffffff', b.x + 4, b.y + 4, 10, b.h - 8);
  R('#dcd7cd', b.x + 4, b.y + b.h - 5, 10, 1);
  R('#7aa2d6', b.x + 15, b.y + 2, b.w - 17, b.h - 4);
  R('#6a90c2', b.x + 15, b.y + 2, 2, b.h - 4);
}

function drawStaticBackground(ctx: CanvasRenderingContext2D, layout: Layout) {
  const W = layout.width;
  const H = layout.height;
  const z = layout.zones;
  const R = filler(ctx);

  R('#1c1e29', 0, 0, W, H);

  // Sàn gỗ khu làm việc
  for (let y = z.backWall.h, row = 0; y < z.work.h; y += 8, row++) {
    R(row % 2 ? '#c79f77' : '#cba47c', 0, y, z.work.w, 8);
    R('#b38b63', 0, y + 7, z.work.w, 1);
    for (let x = (row % 2) * 24; x < z.work.w; x += 48) R('#b38b63', x, y, 1, 7);
  }
  // Nắng từ cửa sổ rọi xuống sàn
  for (let i = 0; i < 36; i++) {
    const spread = Math.floor(i / 3);
    R('rgba(255,246,214,0.16)', z.window.x + 6 - spread, z.backWall.h + i, z.window.w - 12 + spread * 2, 1);
  }
  // Thảm lối đi giữa
  R('#b5886a', z.aisle.x + 6, z.aisle.y, z.aisle.w - 12, z.aisle.h);
  R('#a4775b', z.aisle.x + 6, z.aisle.y, 1, z.aisle.h);
  R('#a4775b', z.aisle.x + z.aisle.w - 7, z.aisle.y, 1, z.aisle.h);

  // Khu chỉ huy (thảm)
  R('#5b4b6e', z.command.x, z.command.y, z.command.w, z.command.h);
  for (let y = z.command.y + 4; y < z.command.y + z.command.h; y += 6)
    for (let x = z.command.x + 3 + ((y / 6) % 2) * 3; x < z.command.x + z.command.w; x += 6) R('#665580', x, y, 1, 1);

  // Phòng nghỉ (gạch) + sảnh
  R('#3d4b5e', z.breakRoom.x, z.breakRoom.y, z.breakRoom.w, z.breakRoom.h);
  for (let y = z.breakRoom.y; y < z.breakRoom.y + z.breakRoom.h; y += 12) R('#35424f', z.breakRoom.x, y, z.breakRoom.w, 1);
  for (let x = z.breakRoom.x; x < z.breakRoom.x + z.breakRoom.w; x += 12) R('#35424f', x, z.breakRoom.y, 1, z.breakRoom.h);
  R('#2c3644', z.breakRoom.x, z.breakRoom.y, z.breakRoom.w, 2);
  R('#4a4358', z.lobby.x, z.lobby.y, z.lobby.w, z.lobby.h);
  R('#2c2836', z.lobby.x, z.lobby.y, z.lobby.w, 2);
  R('#7d5a50', z.lobbyRug.x, z.lobbyRug.y, z.lobbyRug.w, z.lobbyRug.h);
  R('#8f6a5e', z.lobbyRug.x + 2, z.lobbyRug.y + 2, z.lobbyRug.w - 4, z.lobbyRug.h - 4);
  R('#2b2e3b', z.lobby.x + z.lobby.w - 40, z.lobby.y + z.lobby.h - 6, 28, 6);
  R('rgba(190,230,255,0.5)', z.lobby.x + z.lobby.w - 38, z.lobby.y + z.lobby.h - 5, 24, 4);

  // Tường sau
  R('#4a4f6d', 0, 0, W, z.backWall.h);
  R('#5d6385', 0, 0, W, 3);
  R('#3a3e57', 0, z.backWall.h - 4, W, 4);
  // Bảng công việc
  R('#8b8fa3', z.board.x, z.board.y, z.board.w, z.board.h);
  R('#f4f4ef', z.board.x + 1, z.board.y + 1, z.board.w - 2, z.board.h - 2);
  const statusColors = ['#9aa0b5', '#f2a541', '#a99bff', '#5cc98a'];
  const colW = Math.floor((z.board.w - 4) / 4);
  statusColors.forEach((color, i) => {
    R(color, z.board.x + 2 + i * colW, z.board.y + 3, colW - 2, 2);
    for (let k = 0; k < 3 - (i % 2); k++) R('#e3e1d6', z.board.x + 3 + i * colW, z.board.y + 8 + k * 7, colW - 4, 5);
    if (i > 0) R('#dcdcd4', z.board.x + i * colW, z.board.y + 2, 1, z.board.h - 4);
  });
  // Cửa sổ kính ở giữa
  drawWindow(R, z.window);
  // Mảng tường phải: tấm nền logo INOVA
  R('#1a1c27', z.logo.x - 4, z.logo.y - 3, z.logo.w + 8, z.logo.h + 6);
  R('#33374b', z.logo.x - 4, z.logo.y - 3, z.logo.w + 8, 1);
  R('#33374b', z.logo.x - 4, z.logo.y + z.logo.h + 2, z.logo.w + 8, 1);

  // Vách kính khu chỉ huy + cửa
  const doorTop = z.commandDoor.y;
  const doorBottom = z.commandDoor.y + z.commandDoor.h;
  R('rgba(190,230,255,0.45)', z.command.x, z.backWall.h, 3, doorTop - z.backWall.h);
  R('#9fb4cc', z.command.x, z.backWall.h, 1, doorTop - z.backWall.h);
  R('rgba(190,230,255,0.45)', z.command.x, doorBottom, 3, z.command.h - doorBottom);
  R('#9fb4cc', z.command.x, doorBottom, 1, z.command.h - doorBottom);
  R('#7e8aa6', z.command.x - 2, doorTop, 7, 2);
  R('#7e8aa6', z.command.x - 2, doorBottom - 2, 7, 2);

  // Ghế giám đốc sau bàn trưởng phòng và sau bàn khu chỉ huy (bàn vẽ ở lớp trước)
  const ld = z.leadDesk;
  drawExecChair(R, ld.x + Math.floor(ld.w / 2), ld.y - 12);
  for (const ud of layout.userDesks) drawExecChair(R, ud.x + 16, ud.y - 12);

  // Chỗ ngồi nhân viên
  for (const st of layout.seats) drawStaffDesk(R, st);

  // Phòng nghỉ: giường, sofa, quầy cà phê
  for (const b of z.beds) drawBed(R, b);
  const so = z.sofa;
  R('#4f4470', so.x, so.y, so.w, so.h);
  R('#6b5b95', so.x + 2, so.y + 2, so.w - 4, so.h - 6);
  R('#5d4f86', so.x + Math.floor(so.w / 2), so.y + 2, 1, so.h - 6);
  const bx = z.breakRoom.x + z.breakRoom.w - 64;
  R('#9c7653', bx, z.breakRoom.y + 6, 56, 10);
  R('#7a5a3c', bx, z.breakRoom.y + 16, 56, 6);
  R('#2e2e36', bx + 4, z.breakRoom.y - 2, 10, 10);
  R('#e5484d', bx + 10, z.breakRoom.y + 1, 2, 2);
  R('#ffffff', bx + 18, z.breakRoom.y + 4, 4, 3);
  R('#ffffff', bx + 26, z.breakRoom.y + 4, 4, 3);

  // Cây xanh
  drawPlant(ctx, 108, 46);
  drawPlant(ctx, 250, 46);
  drawPlant(ctx, 470, 46);
  drawPlant(ctx, 470, z.command.h - 6);
  drawPlant(ctx, 8, z.work.h - 4);
  drawPlant(ctx, 470, H - 8);
}

function drawForeground(ctx: CanvasRenderingContext2D, layout: Layout) {
  const R = filler(ctx);
  drawExecDeskFront(R, layout.zones.leadDesk, true);
  for (const ud of layout.userDesks) drawExecDeskFront(R, ud, false);
}

let cachedBg: HTMLCanvasElement | null = null;
let cachedLayoutKey = '';

export function renderOffice(
  cv: HTMLCanvasElement,
  layout: Layout,
  state: OfficeState,
  anim: AnimState,
  t: number
) {
  const ctx = cv.getContext('2d');
  if (!ctx) return;
  // Vẽ theo độ phân giải thật của canvas (DEC-44): mọi toạ độ vẫn là toạ độ logic.
  const S = Math.max(1, Math.round(cv.width / layout.width));

  const layoutKey = `${layout.width}x${layout.height}@${S}-${layout.seats.map((x) => x.agentId).join(',')}-${layout.userDesks.length}`;
  if (!cachedBg || cachedLayoutKey !== layoutKey) {
    if (typeof document !== 'undefined') {
      const off = document.createElement('canvas');
      off.width = layout.width * S;
      off.height = layout.height * S;
      const offCtx = off.getContext('2d');
      if (offCtx) {
        offCtx.setTransform(S, 0, 0, S, 0, 0);
        offCtx.imageSmoothingEnabled = false;
        drawStaticBackground(offCtx, layout);
        cachedBg = off;
        cachedLayoutKey = layoutKey;
      }
    }
  }

  ctx.setTransform(1, 0, 0, 1, 0, 0);
  ctx.imageSmoothingEnabled = false;
  if (cachedBg) {
    ctx.drawImage(cachedBg, 0, 0);
  }
  ctx.setTransform(S, 0, 0, S, 0, 0);
  if (!cachedBg) drawStaticBackground(ctx, layout);

  drawClock(ctx, 106, 22);
  const R = filler(ctx);

  // Màn hình nhân viên (mặt màn hình hướng về người ngồi → người xem thấy được)
  for (const s of layout.seats) {
    const cx = s.x + Math.floor(s.w / 2);
    const sx = cx - 8;
    const sy = s.y - 9;
    const char = s.agentId ? state.chars[s.agentId] : null;
    const isDark = s.workerId ? state.dark.has(s.workerId) : false;
    if (!char || char.place === 'offline' || isDark) {
      R('#0d0e12', sx, sy, 16, 9);
    } else if (char.place === 'auth') {
      R('#451a03', sx, sy, 16, 9);
      R('#fbbf24', sx + 6, sy + 3, 4, 4);
      R('#fbbf24', sx + 7, sy + 1, 2, 2);
    } else if (char.task) {
      R('#13261c', sx, sy, 16, 9);
      const ph = Math.floor(t / 180);
      for (let i = 0; i < 4; i++) R(i % 2 ? '#5ad1a0' : '#7fb2ff', sx + 1, sy + 1 + i * 2, 5 + ((ph + i * 3) % 9), 1);
    } else if (char.place === 'break') {
      R('#1a2233', sx, sy, 16, 9);
    } else {
      R('#1e3350', sx, sy, 16, 9);
      R('#4f7fb8', sx + 1 + (Math.floor(t / 500) % 12), sy + 4, 2, 2);
    }
  }

  const drawList: { y: number; draw: () => void }[] = [];
  const bubbles: (() => void)[] = [];

  for (const [id, c] of Object.entries(state.chars)) {
    const a = anim.chars[id];
    if (!a) continue;
    const isOffline = c.place === 'offline' || state.dark.has(c.worker);
    const isClaude = c.kind === 'CLAUDE';
    const look: SpriteProps = {
      shirt: isOffline ? '#55596e' : isClaude ? '#d97757' : '#3fa66b',
      darkShirt: isOffline ? '#3e4252' : isClaude ? '#a64d32' : '#276b45',
      pants: isOffline ? '#3b3e4f' : isClaude ? '#3b3a48' : '#2c3e50',
      hair: isOffline ? '#444' : isClaude ? '#6b4226' : '#1f1f1f',
      skin: isOffline ? '#9a9da8' : isClaude ? '#f3cfa8' : '#e8b88f',
      glasses: isClaude,
      hood: !isClaude,
      facing: a.facing,
      walking: a.walking,
      pose: a.walking ? 'stand' : a.pose,
      typing: !a.walking && (c.place === 'desk' || c.place === 'lead-seat') && !!c.task,
      t,
    };
    // Lead ngồi sau bàn: vẽ trước lớp bàn; còn lại sắp theo y.
    drawList.push({ y: c.place === 'lead-seat' && !a.walking ? -1 : a.y, draw: () => drawSprite(ctx, a.x, a.y, look) });

    if (c.place === 'auth') bubbles.push(() => drawMiniBubble(ctx, a.x, a.y - 26, '🔑 cần đăng nhập'));
    else if (c.place === 'break') bubbles.push(() => drawMiniBubble(ctx, a.x + 8, a.y - 14, c.note ? `💤 ${c.note}` : '💤'));
    else if (c.task) bubbles.push(() => drawMiniBubble(ctx, a.x, a.y - (a.pose === 'sit-back' ? 26 : 30), `#${c.task}`));
  }

  for (const ud of layout.userDesks) {
    const isOnline = state.users[ud.login]?.online ?? ud.online;
    if (!isOnline) continue;
    const ux = ud.x + 16;
    const uy = ud.y + 6;
    drawList.push({
      y: -1,
      draw: () =>
        drawSprite(ctx, ux, uy, {
          shirt: ud.isOwner ? '#c8102e' : '#94a3b8',
          darkShirt: ud.isOwner ? '#990d23' : '#64748b',
          pants: '#1e293b',
          hair: '#334155',
          skin: '#f1c7a0',
          tie: ud.isOwner ? '#fbbf24' : undefined,
          facing: 'front',
          walking: false,
          pose: 'sit-front',
          typing: false,
          t,
        }),
    });
  }

  drawList.sort((p, q) => p.y - q.y);
  // Người ngồi sau bàn (y = -1) → vẽ trước bàn; rồi bàn lớp trước; rồi phần còn lại.
  for (const item of drawList) if (item.y < 0) item.draw();
  drawForeground(ctx, layout);
  for (const item of drawList) if (item.y >= 0) item.draw();
  for (const b of bubbles) b();
}

type SpriteProps = {
  shirt: string;
  darkShirt?: string;
  pants: string;
  hair: string;
  skin: string;
  tie?: string;
  glasses?: boolean;
  hood?: boolean;
  facing: 'front' | 'back';
  walking: boolean;
  pose: Pose;
  typing: boolean;
  t: number;
};

function drawSprite(ctx: CanvasRenderingContext2D, X: number, Y: number, p: SpriteProps) {
  const R = filler(ctx);
  const x = Math.round(X);
  const y = Math.round(Y);
  const ph = Math.floor(p.t / 140) % 2;
  const dark = p.darkShirt || p.shirt;

  if (p.pose === 'lie') {
    // Nằm ngủ trên giường: đầu trên gối (bên trái), chăn màu áo phủ người.
    const hx = x - 10;
    const hy = y - 5;
    R(p.skin, hx, hy, 7, 7);
    R(p.hair, hx, hy - 1, 3, 9);
    R('#222', hx + 4, hy + 3, 2, 1);
    R(p.shirt, x - 2, y - 7, 26, 11);
    R(dark, x - 2, y - 7, 26, 1);
    R(dark, x + 10, y - 6, 1, 9);
    return;
  }

  const sitting = p.pose === 'sit-back' || p.pose === 'sit-front';
  const by = sitting ? y - 3 : y; // ngồi: thân thấp hơn, chân khuất sau ghế/bàn

  if (!sitting) {
    if (p.walking) {
      const l1 = ph ? 1 : 0;
      const l2 = ph ? 0 : 1;
      R(p.pants, x - 4, by - 6, 3, 6 - l1);
      R(p.pants, x + 1, by - 6, 3, 6 - l2);
      R('#222', x - 4, by - 1 - l1, 3, 1);
      R('#222', x + 1, by - 1 - l2, 3, 1);
    } else {
      R(p.pants, x - 4, by - 6, 3, 6);
      R(p.pants, x + 1, by - 6, 3, 6);
      R('#222', x - 4, by - 1, 3, 1);
      R('#222', x + 1, by - 1, 3, 1);
    }
  } else {
    R(p.pants, x - 4, by - 6, 8, 3);
  }

  R(p.shirt, x - 5, by - 14, 10, 8);
  R(dark, x - 6, by - 13, 1, 6);
  R(dark, x + 5, by - 13, 1, 6);

  if (p.typing) {
    R(p.skin, x - 7, by - 12 + ph, 1, 2);
    R(p.skin, x + 6, by - 11 - ph, 1, 2);
  } else {
    R(p.skin, x - 6, by - 7, 1, 1);
    R(p.skin, x + 5, by - 7, 1, 1);
  }

  const hy = by - 21;
  R(p.skin, x - 4, hy, 8, 7);
  if (p.facing === 'back') {
    R(p.hair, x - 4, hy - 1, 8, 6);
    if (p.hood) R(dark, x - 5, hy + 5, 10, 2);
  } else {
    R(p.hair, x - 4, hy - 1, 8, 3);
    R(p.hair, x - 4, hy + 2, 1, 2);
    R(p.hair, x + 3, hy + 2, 1, 2);
    R('#222', x - 2, hy + 3, 1, 1);
    R('#222', x + 1, hy + 3, 1, 1);
    if (p.glasses) {
      R('#222', x - 3, hy + 3, 3, 1);
      R('#222', x + 1, hy + 3, 3, 1);
    }
    if (p.tie) R(p.tie, x, by - 14, 1, 5);
  }
}

function drawMiniBubble(ctx: CanvasRenderingContext2D, x: number, y: number, text: string) {
  // Chữ vẽ ở độ phân giải thật (canvas đã nhân S) → nét; nền đậm, viền sáng cho tương phản.
  ctx.font = '600 7px ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif';
  const width = Math.ceil(ctx.measureText(text).width) + 6;
  const h = 10;
  const bx = Math.round(x - width / 2);
  const by = Math.round(y - h / 2);
  ctx.fillStyle = 'rgba(255,255,255,0.85)';
  ctx.fillRect(bx - 0.5, by - 0.5, width + 1, h + 1);
  ctx.fillStyle = '#111827';
  ctx.fillRect(bx, by, width, h);
  ctx.fillStyle = '#ffffff';
  ctx.textBaseline = 'middle';
  ctx.fillText(text, bx + 3, by + h / 2 + 0.5);
  ctx.textBaseline = 'alphabetic';
}
