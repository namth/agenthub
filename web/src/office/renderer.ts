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
};

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
): { x: number; y: number } {
  if (place === 'lead-seat') {
    const z = layout.zones.leadDesk;
    return { x: z.x + Math.floor(z.w / 2), y: z.y + z.h + 2 };
  }
  if (place === 'break') {
    const z = layout.zones.breakRoom;
    return { x: z.x + 28 + breakIdx * 28, y: z.y + 36 };
  }
  if (place === 'auth') {
    const seat = layout.seats.find((s) => s.agentId === id);
    if (seat) {
      const offsetX = seat.col === 0 ? -12 : seat.w + 12;
      return { x: seat.x + offsetX, y: seat.y + 12 };
    }
  }

  // 'desk' or 'offline'
  const seat = layout.seats.find((s) => s.agentId === id);
  if (seat) {
    return { x: seat.x + Math.floor(seat.w / 2), y: seat.y + seat.h - 4 };
  }

  return { x: 50, y: 100 };
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
        facing: c.place === 'lead-seat' || c.place === 'desk' ? 'back' : 'front',
      };
    } else {
      const cur = anim.chars[id];
      if (Math.hypot(cur.targetX - target.x, cur.targetY - target.y) > 2) {
        cur.targetX = target.x;
        cur.targetY = target.y;
        cur.pts = routeTo({ x: cur.x, y: cur.y }, target, layout);
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

function drawStaticBackground(ctx: CanvasRenderingContext2D, layout: Layout) {
  const W = layout.width;
  const H = layout.height;
  const z = layout.zones;

  const R = (c: string, rx: number, ry: number, rw: number, rh: number) => {
    ctx.fillStyle = c;
    ctx.fillRect(Math.round(rx), Math.round(ry), Math.round(rw), Math.round(rh));
  };

  // 1. Overall base fill
  R('#1c1e29', 0, 0, W, H);

  // 2. Work area wood floor
  for (let y = z.backWall.h, row = 0; y < z.work.h; y += 8, row++) {
    R(row % 2 ? '#c79f77' : '#cba47c', 0, y, z.work.w, 8);
    R('#b38b63', 0, y + 7, z.work.w, 1);
    for (let x = (row % 2) * 24; x < z.work.w; x += 48) {
      R('#b38b63', x, y, 1, 7);
    }
  }

  // 3. Central aisle runner
  R('#c1966a', z.aisle.x, z.aisle.y, z.aisle.w, z.aisle.h);
  R('#b08458', z.aisle.x, z.aisle.y, 1, z.aisle.h);
  R('#b08458', z.aisle.x + z.aisle.w - 1, z.aisle.y, 1, z.aisle.h);

  // 4. Command area floor (carpet style)
  R('#5b4b6e', z.command.x, z.command.y, z.command.w, z.command.h);
  for (let y = z.command.y + 4; y < z.command.y + z.command.h; y += 6) {
    for (
      let x = z.command.x + 3 + ((y / 6) % 2) * 3;
      x < z.command.x + z.command.w;
      x += 6
    ) {
      R('#665580', x, y, 1, 1);
    }
  }

  // 5. Break room floor (tile style)
  R('#3d4b5e', z.breakRoom.x, z.breakRoom.y, z.breakRoom.w, z.breakRoom.h);
  for (let y = z.breakRoom.y; y < z.breakRoom.y + z.breakRoom.h; y += 12) {
    R('#35424f', z.breakRoom.x, y, z.breakRoom.w, 1);
  }
  for (let x = z.breakRoom.x; x < z.breakRoom.x + z.breakRoom.w; x += 12) {
    R('#35424f', x, z.breakRoom.y, 1, z.breakRoom.h);
  }

  // 6. Lobby floor & rug
  R('#4a4358', z.lobby.x, z.lobby.y, z.lobby.w, z.lobby.h);
  R('#7d5a50', z.lobbyRug.x, z.lobbyRug.y, z.lobbyRug.w, z.lobbyRug.h);
  R('#8f6a5e', z.lobbyRug.x + 2, z.lobbyRug.y + 2, z.lobbyRug.w - 4, z.lobbyRug.h - 4);

  // 7. Back wall & trim
  R('#4a4f6d', 0, 0, W, z.backWall.h);
  R('#5d6385', 0, 0, W, 3);
  R('#3a3e57', 0, z.backWall.h - 4, W, 4);

  // Dark backing panel for INOVA logo
  R('#1a1c27', z.logo.x - 4, z.logo.y - 2, z.logo.w + 8, z.logo.h + 4);
  R('#33374b', z.logo.x - 4, z.logo.y - 2, z.logo.w + 8, 1);
  R('#33374b', z.logo.x - 4, z.logo.y + z.logo.h + 1, z.logo.w + 8, 1);

  // Job bulletin board
  R('#8b8fa3', z.board.x, z.board.y, z.board.w, z.board.h);
  R('#f4f4ef', z.board.x + 1, z.board.y + 1, z.board.w - 2, z.board.h - 2);
  const statusColors = ['#9aa0b5', '#f2a541', '#a99bff', '#5cc98a'];
  const colW = Math.floor((z.board.w - 4) / 4);
  statusColors.forEach((color, i) => {
    R(color, z.board.x + 2 + i * colW, z.board.y + 3, colW - 2, 2);
    if (i > 0) {
      R('#dcdcd4', z.board.x + i * colW, z.board.y + 2, 1, z.board.h - 4);
    }
  });

  // 8. Glass partition between work area and command area
  const doorTop = z.commandDoor.y;
  const doorBottom = z.commandDoor.y + z.commandDoor.h;

  // Glass top portion
  R('rgba(190,230,255,0.45)', z.command.x, z.backWall.h, 3, doorTop - z.backWall.h);
  R('#9fb4cc', z.command.x, z.backWall.h, 1, doorTop - z.backWall.h);

  // Glass bottom portion
  R('rgba(190,230,255,0.45)', z.command.x, doorBottom, 3, z.command.h - doorBottom);
  R('#9fb4cc', z.command.x, doorBottom, 1, z.command.h - doorBottom);

  // Glass door frame
  R('#7e8aa6', z.command.x - 2, doorTop, 7, 2);
  R('#7e8aa6', z.command.x - 2, doorBottom - 2, 7, 2);

  // 9. Lead desk
  const ld = z.leadDesk;
  R('#b07a4c', ld.x, ld.y, ld.w, ld.h - 8);
  R('#c48c5a', ld.x, ld.y, ld.w, 2);
  R('#8d5c33', ld.x, ld.y + ld.h - 8, ld.w, 8);
  // Gold star ★ on lead desk
  R('#f5c542', ld.x + Math.floor(ld.w / 2) - 2, ld.y + 4, 4, 4);
  // Executive chair
  R('#2b2e3b', ld.x + Math.floor(ld.w / 2) - 7, ld.y + ld.h + 2, 14, 8);

  // 10. Break room counter & coffee machine
  R('#9c7653', z.breakRoom.x + 8, z.breakRoom.y + 14, 36, 12);
  R('#7a5a3c', z.breakRoom.x + 8, z.breakRoom.y + 24, 36, 8);
  R('#2e2e36', z.breakRoom.x + 12, z.breakRoom.y + 4, 12, 12);
  R('#e5484d', z.breakRoom.x + 20, z.breakRoom.y + 7, 2, 2);
  R('#ffffff', z.breakRoom.x + 28, z.breakRoom.y + 12, 4, 3);

  // 11. Command area user desks
  for (const ud of layout.userDesks) {
    R('#7d5a50', ud.x, ud.y, ud.w, ud.h - 6);
    R('#8f6a5e', ud.x, ud.y, ud.w, 1);
    R('#67483f', ud.x, ud.y + ud.h - 6, ud.w, 6);
    // User monitor
    const mx = ud.x + 14;
    R('#1d1f27', mx - 8, ud.y - 8, 16, 9);
    R('#555', mx - 1, ud.y, 2, 2);
    // Chair
    R('#2b2e3b', mx - 6, ud.y + ud.h - 2, 12, 6);
  }

  // 12. Work area seats and desks
  for (const s of layout.seats) {
    R('#b07a4c', s.x, s.y, s.w, s.h - 6);
    R('#c48c5a', s.x, s.y, s.w, 1);
    R('#8d5c33', s.x, s.y + s.h - 6, s.w, 6);
    R('#6d4426', s.x + 2, s.y + s.h, 2, 3);
    R('#6d4426', s.x + s.w - 4, s.y + s.h, 2, 3);

    // Keyboard
    const cx = s.x + Math.floor(s.w / 2);
    R('#e3e3e3', cx - 5, s.y + 3, 10, 2);

    // Monitor
    R('#1d1f27', cx - 8, s.y - 9, 16, 10);
    R('#555', cx - 1, s.y + 1, 2, 2);

    // Chair
    R('#2b2e3b', cx - 6, s.y + s.h + 2, 12, 6);
    R('#3d4155', cx - 6, s.y + s.h + 2, 12, 1);
  }

  // 13. Decorative plants (Plant / plant)
  drawPlant(ctx, 160, 42);
  drawPlant(ctx, 320, 42);
  drawPlant(ctx, 460, 42);
  drawPlant(ctx, 460, z.command.h - 10);
  drawPlant(ctx, 220, z.breakRoom.y + 24);
  drawPlant(ctx, 460, H - 10);
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

  const layoutKey = `${layout.width}x${layout.height}-${layout.seats.length}-${layout.userDesks.length}`;
  if (!cachedBg || cachedLayoutKey !== layoutKey) {
    if (typeof document !== 'undefined') {
      const off = document.createElement('canvas');
      off.width = layout.width;
      off.height = layout.height;
      const offCtx = off.getContext('2d');
      if (offCtx) {
        drawStaticBackground(offCtx, layout);
        cachedBg = off;
        cachedLayoutKey = layoutKey;
      }
    }
  }

  if (cachedBg) {
    ctx.drawImage(cachedBg, 0, 0);
  } else {
    drawStaticBackground(ctx, layout);
  }

  // Draw clock with live hands
  drawClock(ctx, 150, 22);

  const R = (c: string, rx: number, ry: number, rw: number, rh: number) => {
    ctx.fillStyle = c;
    ctx.fillRect(Math.round(rx), Math.round(ry), Math.round(rw), Math.round(rh));
  };

  // Draw active monitors on work seats
  for (const s of layout.seats) {
    const cx = s.x + Math.floor(s.w / 2);
    const sx = cx - 7;
    const sy = s.y - 8;
    const char = s.agentId ? state.chars[s.agentId] : null;
    const isDark = s.workerId ? state.dark.has(s.workerId) : false;

    if (!char || char.place === 'offline' || isDark) {
      R('#0d0e12', sx, sy, 14, 8); // off
    } else if (char.place === 'auth') {
      R('#451a03', sx, sy, 14, 8); // amber lock
      R('#fbbf24', sx + 5, sy + 2, 4, 4);
    } else if (char.task) {
      R('#13261c', sx, sy, 14, 8);
      const ph = Math.floor(t / 180);
      for (let i = 0; i < 3; i++) {
        R(
          i % 2 ? '#5ad1a0' : '#7fb2ff',
          sx + 1,
          sy + 1 + i * 2,
          6 + ((ph + i * 3) % 7),
          1
        );
      }
    } else {
      R('#1e3350', sx, sy, 14, 8);
      R('#4f7fb8', sx + (Math.floor(t / 500) % 10), sy + 3, 2, 2);
    }
  }

  // Draw characters
  const drawList: { y: number; draw: () => void }[] = [];

  for (const [id, c] of Object.entries(state.chars)) {
    const a = anim.chars[id];
    if (!a) continue;

    const isOffline = c.place === 'offline' || state.dark.has(c.worker);
    const isClaude = c.kind === 'CLAUDE';
    const shirt = isOffline ? '#55596e' : isClaude ? '#d97757' : '#3fa66b';
    const darkShirt = isOffline ? '#3e4252' : isClaude ? '#a64d32' : '#276b45';
    const pants = isOffline ? '#3b3e4f' : isClaude ? '#3b3a48' : '#2c3e50';
    const hair = isOffline ? '#444' : isClaude ? '#6b4226' : '#1f1f1f';
    const skin = isOffline ? '#9a9da8' : isClaude ? '#f3cfa8' : '#e8b88f';
    const glasses = isClaude;
    const hood = !isClaude;
    const typing = !a.walking && (c.place === 'desk' || c.place === 'lead-seat') && !!c.task;

    drawList.push({
      y: a.y,
      draw: () => {
        drawSprite(ctx, a.x, a.y, {
          shirt,
          darkShirt,
          pants,
          hair,
          skin,
          glasses,
          hood,
          facing: a.facing,
          walking: a.walking,
          typing,
          t,
        });

        if (c.place === 'auth') {
          drawMiniBubble(ctx, a.x, a.y - 20, '🔑');
        } else if (c.place === 'break' && c.note) {
          drawMiniBubble(ctx, a.x, a.y - 18, `☕ ${c.note}`);
        } else if (c.task) {
          drawMiniBubble(ctx, a.x, a.y - 18, `#${c.task}`);
        }
      },
    });
  }

  // Draw users in command area
  for (const ud of layout.userDesks) {
    const isOnline = state.users[ud.login]?.online ?? ud.online;
    if (isOnline) {
      const shirt = ud.isOwner ? '#c8102e' : '#64748b';
      const darkShirt = ud.isOwner ? '#990d23' : '#475569';
      const ux = ud.x + 14;
      const uy = ud.y + ud.h + 2;

      drawList.push({
        y: uy,
        draw: () => {
          drawSprite(ctx, ux, uy, {
            shirt,
            darkShirt,
            pants: '#1e293b',
            hair: '#334155',
            skin: '#f1c7a0',
            tie: ud.isOwner ? '#fbbf24' : undefined,
            facing: 'front',
            walking: false,
            typing: false,
            t,
          });
        },
      });
    }
  }

  drawList.sort((a, b) => a.y - b.y);
  for (const item of drawList) {
    item.draw();
  }
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
  typing: boolean;
  t: number;
};

function drawSprite(ctx: CanvasRenderingContext2D, X: number, Y: number, p: SpriteProps) {
  const R = (color: string, x: number, y: number, w: number, h: number) => {
    ctx.fillStyle = color;
    ctx.fillRect(Math.round(x), Math.round(y), Math.round(w), Math.round(h));
  };

  const x = Math.round(X);
  const y = Math.round(Y);
  const ph = Math.floor(p.t / 140) % 2;
  const dark = p.darkShirt || p.shirt;

  // Legs / pants
  if (p.walking) {
    const l1 = ph ? 1 : 0;
    const l2 = ph ? 0 : 1;
    R(p.pants, x - 4, y - 6, 3, 6 - l1);
    R(p.pants, x + 1, y - 6, 3, 6 - l2);
    R('#222', x - 4, y - 1 - l1, 3, 1);
    R('#222', x + 1, y - 1 - l2, 3, 1);
  } else {
    R(p.pants, x - 4, y - 6, 3, 6);
    R(p.pants, x + 1, y - 6, 3, 6);
    R('#222', x - 4, y - 1, 3, 1);
    R('#222', x + 1, y - 1, 3, 1);
  }

  // Torso / shirt
  R(p.shirt, x - 5, y - 14, 10, 8);
  R(dark, x - 6, y - 13, 1, 6);
  R(dark, x + 5, y - 13, 1, 6);

  // Arms / hands
  if (p.typing) {
    R(p.skin, x - 7, y - 12 + ph, 1, 2);
    R(p.skin, x + 6, y - 11 - ph, 1, 2);
  } else {
    R(p.skin, x - 6, y - 7, 1, 1);
    R(p.skin, x + 5, y - 7, 1, 1);
  }

  // Head
  const hy = y - 21;
  R(p.skin, x - 4, hy, 8, 7);

  // Hair & Face
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
    if (p.tie) {
      R(p.tie, x, y - 14, 1, 5);
    }
  }
}

function drawMiniBubble(ctx: CanvasRenderingContext2D, x: number, y: number, text: string) {
  ctx.font = '8px sans-serif';
  const width = ctx.measureText(text).width + 6;
  const h = 11;
  const bx = Math.round(x - width / 2);
  const by = Math.round(y - h / 2);

  ctx.fillStyle = '#000000cc';
  ctx.fillRect(bx, by, width, h);
  ctx.fillStyle = '#ffffff';
  ctx.fillText(text, bx + 3, by + 8);
}
