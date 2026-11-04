import type { computeLayout, OfficeState, Place } from './model';

type Layout = ReturnType<typeof computeLayout>;

export type CharAnim = {
  x: number;
  y: number;
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
  workerId: string,
  layout: Layout,
  breakIdx: number
): { x: number; y: number } {
  if (place === 'lead-seat') {
    const z = layout.zones.leadSeat;
    return { x: z.x + Math.floor(z.w / 2), y: z.y + 36 };
  }
  if (place === 'break') {
    const z = layout.zones.breakRoom;
    return { x: z.x + 24 + breakIdx * 30, y: z.y + 36 };
  }

  // 'desk' or 'offline'
  for (const row of layout.rows) {
    if (row.workerId === workerId) {
      const d = row.desks.find((item) => item.agentId === id);
      if (d) {
        return { x: d.x + Math.floor(d.w / 2), y: d.y + 14 };
      }
    }
  }

  return { x: 50, y: 100 };
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
        targetX: target.x,
        targetY: target.y,
        walking: false,
        facing: c.place === 'lead-seat' || c.place === 'desk' ? 'back' : 'front',
      };
    } else {
      anim.chars[id].targetX = target.x;
      anim.chars[id].targetY = target.y;
    }
  }
}

export function stepAnim(anim: AnimState, dt: number): boolean {
  let moving = false;
  const speed = 64; // px/s

  for (const c of Object.values(anim.chars)) {
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

  return moving;
}

export function renderOffice(
  cv: HTMLCanvasElement,
  layout: Layout,
  state: OfficeState,
  anim: AnimState,
  t: number
) {
  const ctx = cv.getContext('2d');
  if (!ctx) return;

  const W = layout.width;
  const H = layout.height;

  // Clear / base floor
  ctx.fillStyle = '#1c1e29';
  ctx.fillRect(0, 0, W, H);

  // Helper rect function
  const R = (color: string, x: number, y: number, w: number, h: number) => {
    ctx.fillStyle = color;
    ctx.fillRect(Math.round(x), Math.round(y), Math.round(w), Math.round(h));
  };

  // Top zones area background (walls and floors)
  R('#3a3e57', 0, 0, W, 68);
  R('#2a2d40', 0, 0, W, 4);

  // 1. Director Zone
  const dz = layout.zones.director;
  R('#5b4b6e', dz.x, dz.y, dz.w, dz.h);
  R('#6d5885', dz.x + 2, dz.y + 2, dz.w - 4, dz.h - 4);
  // Director Desk
  R('#7d5a50', dz.x + 28, dz.y + 20, 40, 16);
  R('#8f6a5e', dz.x + 28, dz.y + 20, 40, 3);
  R('#f5d76e', dz.x + 30, dz.y + 16, 5, 4); // lamp
  // Director person (Anh Nam)
  drawSprite(ctx, dz.x + 48, dz.y + 28, {
    shirt: '#2f3550',
    pants: '#1c1e29',
    hair: '#8d8d8d',
    skin: '#f1c7a0',
    tie: '#c0392b',
    facing: 'front',
    walking: false,
    typing: false,
    t,
  });
  // Label
  ctx.fillStyle = '#e9e7f1';
  ctx.font = '9px monospace';
  ctx.fillText('Giám đốc', dz.x + 4, dz.y + 11);

  // 2. Lead Seat Zone
  const lz = layout.zones.leadSeat;
  R('#424765', lz.x, lz.y, lz.w, lz.h);
  R('#4f5578', lz.x + 2, lz.y + 2, lz.w - 4, lz.h - 4);
  // Lead Desk with Star
  R('#b07a4c', lz.x + 28, lz.y + 20, 40, 16);
  R('#c48c5a', lz.x + 28, lz.y + 20, 40, 3);
  R('#f5c542', lz.x + 46, lz.y + 24, 4, 4); // ★
  // Chair
  R('#2b2e3b', lz.x + 42, lz.y + 36, 12, 6);
  // Label
  ctx.fillStyle = '#f5c542';
  ctx.font = '9px monospace';
  ctx.fillText('Trưởng phòng ★', lz.x + 4, lz.y + 11);
  if (!state.lead) {
    ctx.fillStyle = '#9a9db4';
    ctx.font = '8px monospace';
    ctx.fillText('Ghế trống', lz.x + 28, lz.y + 48);
  }

  // 3. Break Room Zone
  const bz = layout.zones.breakRoom;
  R('#3d4b5e', bz.x, bz.y, bz.w, bz.h);
  R('#48586e', bz.x + 2, bz.y + 2, bz.w - 4, bz.h - 4);
  // Coffee counter & machine
  R('#9c7653', bz.x + 6, bz.y + 16, 26, 10);
  R('#2e2e36', bz.x + 8, bz.y + 8, 10, 10);
  R('#e5484d', bz.x + 15, bz.y + 11, 2, 2); // coffee light
  R('#fff', bz.x + 24, bz.y + 14, 4, 3); // cup
  // Label
  ctx.fillStyle = '#e9e7f1';
  ctx.font = '9px monospace';
  ctx.fillText('Phòng nghỉ ☕', bz.x + 4, bz.y + 11);

  // 4. Worker rows and desks
  for (const row of layout.rows) {
    const isDark = state.dark.has(row.workerId);

    for (const d of row.desks) {
      const char = state.chars[d.agentId];
      const isWorking = char && char.task && !isDark && char.place === 'desk';
      const isOff = isDark || char?.place === 'offline';

      // Floor under desk
      R(isDark ? '#232636' : '#cba47c', d.x - 4, d.y - 14, d.w + 8, d.h + 24);
      if (!isDark) {
        R('#b38b63', d.x - 4, d.y + d.h + 9, d.w + 8, 1);
      }

      // Desk surface & legs
      R(isDark ? '#4a382c' : '#b07a4c', d.x, d.y, d.w, 9);
      R(isDark ? '#5c4637' : '#c48c5a', d.x, d.y, d.w, 1);
      R(isDark ? '#3d2e24' : '#8d5c33', d.x, d.y + 9, d.w, 5);
      R('#4a382c', d.x + 2, d.y + 14, 2, 4);
      R('#4a382c', d.x + d.w - 4, d.y + 14, 2, 4);

      // Keyboard
      const cx = d.x + Math.floor(d.w / 2);
      R('#e3e3e3', cx - 5, d.y + 2, 10, 2);

      // Monitor
      R('#1d1f27', cx - 8, d.y - 10, 16, 10);
      R('#555', cx - 1, d.y, 2, 2);

      // Screen contents
      const sx = cx - 7;
      const sy = d.y - 9;
      if (isOff) {
        R('#0d0e12', sx, sy, 14, 8); // off
      } else if (isWorking) {
        R('#13261c', sx, sy, 14, 8);
        const ph = Math.floor(t / 180);
        for (let i = 0; i < 3; i++) {
          R(i % 2 ? '#5ad1a0' : '#7fb2ff', sx + 1, sy + 1 + i * 2, 6 + ((ph + i * 3) % 7), 1);
        }
      } else if (char?.place === 'desk') {
        R('#1e3350', sx, sy, 14, 8);
        R('#4f7fb8', sx + (Math.floor(t / 500) % 10), sy + 3, 2, 2);
      } else {
        R('#151822', sx, sy, 14, 8);
      }

      // Chair
      R('#2b2e3b', cx - 5, d.y + 16, 10, 6);

      // Desk label / badge
      ctx.fillStyle = isDark ? '#7a8099' : '#33374b';
      ctx.font = '7px monospace';
      const shortId = d.agentId.split('/')[1] || d.agentId;
      ctx.fillText(shortId, d.x, d.y + d.h + 8);
    }
  }

  // 5. Draw Characters sorted by Y for correct layering
  const charDrawList: { y: number; draw: () => void }[] = [];

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
    const typing = !a.walking && c.place === 'desk' && !!c.task;

    charDrawList.push({
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

        // Bubble / Tag
        if (c.place === 'break' && c.note) {
          drawMiniBubble(ctx, a.x, a.y - 18, `☕ ${c.note}`);
        } else if (c.task) {
          drawMiniBubble(ctx, a.x, a.y - 18, `#${c.task}`);
        }
      },
    });
  }

  charDrawList.sort((a, b) => a.y - b.y);
  for (const item of charDrawList) {
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
