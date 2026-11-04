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
    agents: {
      id: string;
      kind: string;
      status: string;
      cooldown_until?: string | null;
      task?: string;
      job?: { task?: number | string; kind?: string };
    }[];
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

export type Place = 'desk' | 'lead-seat' | 'break' | 'offline';

export type OfficeState = {
  lead: string | null;
  vacantNote?: string;
  dark: Set<string>;
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

export function computeLayout(workers: Snapshot['workers']): {
  width: number;
  height: number;
  zones: { director: Rect; leadSeat: Rect; breakRoom: Rect };
  rows: { workerId: string; desks: (Rect & { agentId: string })[] }[];
} {
  const sortedWorkers = [...workers].sort((a, b) => a.id.localeCompare(b.id));

  const width = 384;
  const cols = 2;
  const numWorkers = sortedWorkers.length;
  const numGridRows = Math.ceil(numWorkers / cols);
  const baseHeight = 272;
  const extraRows = Math.max(0, numGridRows - 4);
  const height = baseHeight + extraRows * 50;

  const zones = {
    director: { x: 8, y: 8, w: 96, h: 56 },
    leadSeat: { x: 136, y: 8, w: 96, h: 56 },
    breakRoom: { x: 264, y: 8, w: 112, h: 56 },
  };

  const rows = sortedWorkers.map((worker, wIdx) => {
    const gridCol = wIdx % cols;
    const gridRow = Math.floor(wIdx / cols);
    const colWidth = 192;
    const baseX = gridCol * colWidth;
    const baseY = 72 + gridRow * 50;

    const numAgents = worker.agents.length;
    const deskW =
      numAgents > 0
        ? Math.min(38, Math.max(20, Math.floor((colWidth - 28) / numAgents) - 4))
        : 38;
    const spacing = deskW + 6;

    const desks = worker.agents.map((agent, aIdx) => {
      const x = baseX + 14 + aIdx * spacing;
      const y = baseY + 14;
      return {
        agentId: agent.id,
        x,
        y,
        w: deskW,
        h: 24,
      };
    });

    return {
      workerId: worker.id,
      desks,
    };
  });

  return {
    width,
    height,
    zones,
    rows,
  };
}

export function initialState(s: Snapshot): OfficeState {
  const dark = new Set<string>();
  const lead = s.lead && s.lead.state === 'HELD' ? s.lead.agent_id : null;
  const vacantNote =
    s.lead && s.lead.state === 'VACANT'
      ? s.lead.vacant_note ?? (s.lead as unknown as { text?: string }).text
      : undefined;
  const chars: OfficeState['chars'] = {};

  for (const w of s.workers) {
    const isOffline = w.conn_status === 'OFFLINE';
    if (isOffline) {
      dark.add(w.id);
    }
    for (const a of w.agents) {
      let place: Place = 'desk';
      let note: string | undefined;

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
    chars,
  };
}

export function applyEvent(s: OfficeState, e: OfficeEvent): OfficeState {
  const nextDark = new Set(s.dark);
  const nextChars: OfficeState['chars'] = {};
  for (const [k, v] of Object.entries(s.chars)) {
    nextChars[k] = { ...v };
  }
  let nextLead = s.lead;
  let nextVacantNote = s.vacantNote;

  switch (e.type) {
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
        chars: s.chars,
      };
    }
  }

  return {
    lead: nextLead,
    vacantNote: nextVacantNote,
    dark: nextDark,
    chars: nextChars,
  };
}
