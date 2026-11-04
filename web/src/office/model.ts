export type Rect = {
  x: number;
  y: number;
  w: number;
  h: number;
};

export type Snapshot = {
  lead: {
    state: string;
    agent_id: string | null;
    epoch: number;
    vacant_note?: string;
  };
  workers: {
    id: string;
    conn_status: string;
    gh_auth?: string;
    owner?: string | null;
    agents: {
      id: string;
      kind: string;
      status: string;
      auth_status?: string;
      cooldown_until?: string | null;
      task?: string;
      job?: { task?: number | string; kind?: string };
    }[];
  }[];
  users?: {
    login: string;
    avatar_url: string;
    is_owner: boolean;
    online: boolean;
  }[];
  recent: OfficeEvent[];
};

export type OfficeEvent = {
  id: number;
  ts: string;
  who: string;
  type: string;
  to?: string;
  task?: string;
  text?: string;
};

export type Place = 'desk' | 'lead-seat' | 'break' | 'offline' | 'auth';

export type OfficeState = {
  lead: string | null;
  vacantNote?: string;
  dark: Set<string>;
  ghNeeds: Set<string>;
  users: Record<string, { online: boolean; isOwner: boolean }>;
  chars: Record<
    string,
    {
      worker: string;
      kind: string;
      place: Place;
      task?: string;
      note?: string;
    }
  >;
};

export type Seat = Rect & {
  index: number;
  cluster: 'left' | 'right';
  row: number;
  col: 0 | 1;
  agentId: string | null;
  workerId: string | null;
};

export type UserDesk = Rect & {
  login: string;
  isOwner: boolean;
  online: boolean;
};

export type Zones = {
  backWall: Rect;
  logo: Rect;
  board: Rect;
  leadDesk: Rect;
  work: Rect;
  aisle: Rect;
  command: Rect;
  commandDoor: Rect;
  breakRoom: Rect;
  lobby: Rect;
  lobbyRug: Rect;
};

function getAgentPriority(kind: string): number {
  if (kind === 'CLAUDE') return 1;
  if (kind === 'ANTIGRAVITY') return 2;
  return 3;
}

function getSeatRect(cluster: 'left' | 'right', row: number, col: 0 | 1): Rect {
  const x = cluster === 'left' ? (col === 0 ? 24 : 96) : (col === 0 ? 208 : 280);
  const y = 80 + row * 44;
  return { x, y, w: 56, h: 26 };
}

export function computeLayout(
  workers: Snapshot['workers'],
  users: NonNullable<Snapshot['users']> = []
): {
  width: number;
  height: number;
  zones: Zones;
  seats: Seat[];
  machineLabels: { workerId: string; x: number; y: number }[];
  userDesks: UserDesk[];
} {
  const width = 480;
  const sortedWorkers = [...workers].sort((a, b) =>
    a.id.toLowerCase().localeCompare(b.id.toLowerCase())
  );

  let leftNextRow = 0;
  let rightNextRow = 0;

  const seats: Seat[] = [];
  const machineLabels: { workerId: string; x: number; y: number }[] = [];

  for (const w of sortedWorkers) {
    const sortedAgents = [...w.agents].sort(
      (a, b) =>
        getAgentPriority(a.kind) - getAgentPriority(b.kind) ||
        a.id.localeCompare(b.id)
    );

    const rowsNeeded = Math.max(1, Math.ceil(sortedAgents.length / 2));
    const cluster: 'left' | 'right' = leftNextRow <= rightNextRow ? 'left' : 'right';
    const startRow = cluster === 'left' ? leftNextRow : rightNextRow;

    if (cluster === 'left') {
      leftNextRow += rowsNeeded;
    } else {
      rightNextRow += rowsNeeded;
    }

    for (let r = 0; r < rowsNeeded; r++) {
      const currentRow = startRow + r;
      const a0 = sortedAgents[r * 2] ?? null;
      const a1 = sortedAgents[r * 2 + 1] ?? null;

      const r0 = getSeatRect(cluster, currentRow, 0);
      seats.push({
        ...r0,
        index: seats.length,
        cluster,
        row: currentRow,
        col: 0,
        agentId: a0 ? a0.id : null,
        workerId: a0 ? w.id : null,
      });

      const r1 = getSeatRect(cluster, currentRow, 1);
      seats.push({
        ...r1,
        index: seats.length,
        cluster,
        row: currentRow,
        col: 1,
        agentId: a1 ? a1.id : null,
        workerId: a1 ? w.id : null,
      });
    }

    machineLabels.push({
      workerId: w.id,
      x: cluster === 'left' ? 24 : 208,
      y: 80 + startRow * 44 + 28,
    });
  }

  // Minimum 2 rows per cluster
  const leftTotalRows = Math.max(2, leftNextRow);
  const rightTotalRows = Math.max(2, rightNextRow);

  for (let r = leftNextRow; r < leftTotalRows; r++) {
    const r0 = getSeatRect('left', r, 0);
    seats.push({
      ...r0,
      index: seats.length,
      cluster: 'left',
      row: r,
      col: 0,
      agentId: null,
      workerId: null,
    });
    const r1 = getSeatRect('left', r, 1);
    seats.push({
      ...r1,
      index: seats.length,
      cluster: 'left',
      row: r,
      col: 1,
      agentId: null,
      workerId: null,
    });
  }

  for (let r = rightNextRow; r < rightTotalRows; r++) {
    const r0 = getSeatRect('right', r, 0);
    seats.push({
      ...r0,
      index: seats.length,
      cluster: 'right',
      row: r,
      col: 0,
      agentId: null,
      workerId: null,
    });
    const r1 = getSeatRect('right', r, 1);
    seats.push({
      ...r1,
      index: seats.length,
      cluster: 'right',
      row: r,
      col: 1,
      agentId: null,
      workerId: null,
    });
  }

  // Re-assign continuous indices
  seats.forEach((s, idx) => {
    s.index = idx;
  });

  const maxSeatRows = Math.max(leftTotalRows, rightTotalRows);
  const seatsBottom = 80 + maxSeatRows * 44 + 16;
  const userStartY = 56;
  const userDeskStep = 26;
  const userDeskH = 22;
  const usersBottom =
    users.length > 0 ? userStartY + users.length * userDeskStep + 10 : 0;

  const baseBottomY = 196;
  const bottomY = Math.max(baseBottomY, seatsBottom, usersBottom);
  const bottomH = 64;
  const height = bottomY + bottomH;

  const zones: Zones = {
    backWall: { x: 0, y: 0, w: width, h: 48 },
    logo: { x: 180, y: 8, w: 120, h: 32 },
    board: { x: 16, y: 8, w: 130, h: 32 },
    leadDesk: { x: 170, y: 46, w: 60, h: 26 },
    work: { x: 0, y: 0, w: 360, h: bottomY },
    aisle: { x: 166, y: 72, w: 28, h: bottomY - 72 },
    command: { x: 360, y: 0, w: 120, h: bottomY },
    commandDoor: { x: 356, y: 52, w: 8, h: 24 },
    breakRoom: { x: 0, y: bottomY, w: 240, h: bottomH },
    lobby: { x: 240, y: bottomY, w: 240, h: bottomH },
    lobbyRug: { x: 290, y: bottomY + 12, w: 90, h: 40 },
  };

  const userDesks: UserDesk[] = users.map((u, i) => ({
    login: u.login,
    isOwner: u.is_owner,
    online: u.online,
    x: 375,
    y: userStartY + i * userDeskStep,
    w: 90,
    h: userDeskH,
  }));

  return {
    width,
    height,
    zones,
    seats,
    machineLabels,
    userDesks,
  };
}

export function initialState(s: Snapshot): OfficeState {
  const dark = new Set<string>();
  const ghNeeds = new Set<string>();
  const lead = s.lead && s.lead.state === 'HELD' ? s.lead.agent_id : null;
  const vacantNote =
    s.lead && s.lead.state === 'VACANT'
      ? s.lead.vacant_note ?? (s.lead as unknown as { text?: string }).text
      : undefined;
  const chars: OfficeState['chars'] = {};
  const users: OfficeState['users'] = {};

  if (s.users) {
    for (const u of s.users) {
      users[u.login] = { online: u.online, isOwner: u.is_owner };
    }
  }

  for (const w of s.workers) {
    const isOffline = w.conn_status === 'OFFLINE';
    if (isOffline) {
      dark.add(w.id);
    }
    if (w.gh_auth === 'NEEDS_AUTH') {
      ghNeeds.add(w.id);
    }
    for (const a of w.agents) {
      let place: Place = 'desk';
      let note: string | undefined;

      // Priority: offline > lead-seat > break > auth (NEEDS_AUTH) > desk
      if (isOffline) {
        place = 'offline';
      } else if (lead && lead === a.id) {
        place = 'lead-seat';
      } else if (a.status === 'COOLDOWN') {
        place = 'break';
        if (a.cooldown_until) {
          const d = new Date(a.cooldown_until);
          if (!isNaN(d.getTime())) {
            const hh = String(d.getHours()).padStart(2, '0');
            const mm = String(d.getMinutes()).padStart(2, '0');
            note = `${hh}:${mm}`;
          } else {
            const m = a.cooldown_until.match(/\d{2}:\d{2}/);
            note = m ? m[0] : a.cooldown_until;
          }
        }
      } else if (a.auth_status === 'NEEDS_AUTH') {
        place = 'auth';
      }

      const task =
        a.task ??
        (a.job?.task !== undefined && a.job?.task !== null
          ? String(a.job.task)
          : undefined);

      chars[a.id] = {
        worker: w.id,
        kind: a.kind,
        place,
        task,
        note,
      };
    }
  }

  return {
    lead,
    vacantNote,
    dark,
    ghNeeds,
    users,
    chars,
  };
}

export function applyEvent(s: OfficeState, e: OfficeEvent): OfficeState {
  const nextDark = new Set(s.dark);
  const nextGhNeeds = new Set(s.ghNeeds);
  const nextUsers: OfficeState['users'] = {};
  for (const [k, v] of Object.entries(s.users)) {
    nextUsers[k] = { ...v };
  }
  const nextChars: OfficeState['chars'] = {};
  for (const [k, v] of Object.entries(s.chars)) {
    nextChars[k] = { ...v };
  }
  let nextLead = s.lead;
  let nextVacantNote = s.vacantNote;

  switch (e.type) {
    case 'auth': {
      if (e.who && nextChars[e.who]) {
        if (e.text === 'NEEDS_AUTH') {
          nextChars[e.who] = { ...nextChars[e.who], place: 'auth' };
        } else if (e.text === 'OK') {
          if (nextChars[e.who].place === 'auth') {
            nextChars[e.who] = { ...nextChars[e.who], place: 'desk' };
          }
        }
      } else if (e.who) {
        if (e.text === 'NEEDS_AUTH') {
          nextGhNeeds.add(e.who);
        } else if (e.text === 'OK') {
          nextGhNeeds.delete(e.who);
        }
      }
      break;
    }
    case 'presence': {
      if (e.who) {
        const login = e.who.startsWith('user:') ? e.who.slice(5) : e.who;
        const online = e.text === 'online';
        if (nextUsers[login]) {
          nextUsers[login] = { ...nextUsers[login], online };
        } else {
          nextUsers[login] = { online, isOwner: false };
        }
      }
      break;
    }
    case 'handover': {
      if (e.to && nextChars[e.to]) {
        if (nextLead && nextChars[nextLead] && nextChars[nextLead].place === 'lead-seat') {
          nextChars[nextLead] = { ...nextChars[nextLead], place: 'desk' };
        }
        if (e.who && nextChars[e.who] && nextChars[e.who].place === 'lead-seat') {
          nextChars[e.who] = { ...nextChars[e.who], place: 'desk' };
        }
        nextLead = e.to;
        nextVacantNote = undefined;
        nextChars[e.to] = { ...nextChars[e.to], place: 'lead-seat' };
      }
      break;
    }
    case 'vacant': {
      nextLead = null;
      nextVacantNote = e.text;
      for (const [id, c] of Object.entries(nextChars)) {
        if (c.place === 'lead-seat') {
          nextChars[id] = { ...c, place: 'desk' };
        }
      }
      break;
    }
    case 'cooldown': {
      if (e.who && nextChars[e.who]) {
        nextChars[e.who] = {
          ...nextChars[e.who],
          place: 'break',
          note: e.text || nextChars[e.who].note,
        };
      }
      break;
    }
    case 'idle': {
      if (e.who && nextChars[e.who]) {
        const place: Place = nextLead === e.who ? 'lead-seat' : 'desk';
        nextChars[e.who] = {
          ...nextChars[e.who],
          place,
          note: undefined,
        };
      }
      break;
    }
    case 'assign': {
      const target = e.to;
      if (target && nextChars[target]) {
        nextChars[target] = {
          ...nextChars[target],
          task: e.task,
        };
      }
      break;
    }
    case 'submit': {
      if (e.who && nextChars[e.who]) {
        const updated = { ...nextChars[e.who] };
        delete updated.task;
        nextChars[e.who] = updated;
      }
      break;
    }
    case 'offline': {
      if (e.who) {
        nextDark.add(e.who);
        for (const [id, c] of Object.entries(nextChars)) {
          if (c.worker === e.who) {
            nextChars[id] = { ...c, place: 'offline' };
          }
        }
      }
      break;
    }
    case 'online': {
      if (e.who) {
        nextDark.delete(e.who);
        for (const [id, c] of Object.entries(nextChars)) {
          if (c.worker === e.who) {
            const place: Place =
              id === nextLead
                ? 'lead-seat'
                : c.note && c.place === 'break'
                  ? 'break'
                  : 'desk';
            nextChars[id] = { ...c, place };
          }
        }
      }
      break;
    }
    case 'work': {
      if (e.who && nextChars[e.who] && e.task) {
        nextChars[e.who] = {
          ...nextChars[e.who],
          task: e.task,
        };
      }
      break;
    }
    default: {
      return {
        lead: s.lead,
        vacantNote: s.vacantNote,
        dark: s.dark,
        ghNeeds: s.ghNeeds,
        users: s.users,
        chars: s.chars,
      };
    }
  }

  return {
    lead: nextLead,
    vacantNote: nextVacantNote,
    dark: nextDark,
    ghNeeds: nextGhNeeds,
    users: nextUsers,
    chars: nextChars,
  };
}
