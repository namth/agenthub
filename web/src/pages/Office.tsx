import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router';
import { Empty, ErrorBanner, Loading } from '../components/StateView';
import {
  applyEvent,
  computeLayout,
  initialState,
  type OfficeEvent,
  type OfficeState,
  type Snapshot,
} from '../office/model';
import {
  createAnimState,
  renderOffice,
  stepAnim,
  syncAnimState,
  type AnimState,
} from '../office/renderer';

export default function Office() {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [officeState, setOfficeState] = useState<OfficeState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isPaused, setIsPaused] = useState(false);
  const [logoError, setLogoError] = useState(false);
  const [hoveredInfo, setHoveredInfo] = useState<{
    x: number;
    y: number;
    text: string;
  } | null>(null);

  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const animRef = useRef<AnimState>(createAnimState());
  const isInitialSnap = useRef(true);

  // SSE stream
  useEffect(() => {
    const es = new EventSource('/api/office/stream');

    es.onopen = () => {
      setError(null);
    };

    es.onerror = () => {
      setError('Mất kết nối — đang thử lại');
    };

    es.addEventListener('snapshot', (e: MessageEvent) => {
      try {
        const snap: Snapshot = JSON.parse(e.data);
        isInitialSnap.current = true;
        setSnapshot(snap);
        setOfficeState(initialState(snap));
      } catch (err) {
        console.error('Failed to parse snapshot', err);
      }
    });

    es.addEventListener('office', (e: MessageEvent) => {
      try {
        const ev: OfficeEvent = JSON.parse(e.data);
        setOfficeState((prev) => (prev ? applyEvent(prev, ev) : prev));
      } catch (err) {
        console.error('Failed to parse office event', err);
      }
    });

    return () => {
      es.close();
    };
  }, []);

  const layout = useMemo(() => {
    if (!snapshot) return null;
    return computeLayout(snapshot.workers, snapshot.users ?? []);
  }, [snapshot]);

  // Sync animation positions
  useEffect(() => {
    if (officeState && layout) {
      syncAnimState(
        animRef.current,
        officeState,
        layout,
        isInitialSnap.current
      );
      isInitialSnap.current = false;
      if (canvasRef.current) {
        renderOffice(canvasRef.current, layout, officeState, animRef.current, Date.now());
      }
    }
  }, [officeState, layout]);

  // Canvas render & animation loop
  useEffect(() => {
    let reqId: number;

    const loop = () => {
      if (document.hidden) {
        reqId = requestAnimationFrame(loop);
        return;
      }

      const now = Date.now();
      const anim = animRef.current;
      const dt = Math.min((now - anim.lastTime) / 1000, 0.1);
      anim.lastTime = now;

      let hasActivity = false;
      if (!isPaused && officeState && layout) {
        const moving = stepAnim(anim, dt);
        const working = Object.values(officeState.chars).some(
          (c) => c.task && (c.place === 'desk' || c.place === 'lead-seat')
        );
        hasActivity = moving || working;
      }

      const targetFps = hasActivity ? 12 : 4;
      const interval = 1000 / targetFps;

      if (now - anim.lastFrameTime >= interval) {
        anim.lastFrameTime = now;
        if (canvasRef.current && layout && officeState) {
          renderOffice(canvasRef.current, layout, officeState, anim, now);
        }
      }

      reqId = requestAnimationFrame(loop);
    };

    reqId = requestAnimationFrame(loop);

    return () => {
      cancelAnimationFrame(reqId);
    };
  }, [isPaused, officeState, layout]);

  const handleMouseMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
    if (!canvasRef.current || !layout || !officeState) return;
    const rect = canvasRef.current.getBoundingClientRect();
    const scaleX = layout.width / rect.width;
    const scaleY = layout.height / rect.height;
    const cx = (e.clientX - rect.left) * scaleX;
    const cy = (e.clientY - rect.top) * scaleY;

    for (const [id, c] of Object.entries(officeState.chars)) {
      const a = animRef.current.chars[id];
      if (a && Math.hypot(a.x - cx, a.y - cy) < 18) {
        setHoveredInfo({
          x: e.clientX - rect.left,
          y: e.clientY - rect.top,
          text: `${id} · ${c.place}${c.task ? ` · Task #${c.task}` : ''}${
            c.note ? ` · (${c.note})` : ''
          }`,
        });
        return;
      }
    }

    setHoveredInfo(null);
  };

  const handleMouseLeave = () => {
    setHoveredInfo(null);
  };

  if (!snapshot) {
    return (
      <section className="space-y-4" aria-busy="true">
        <h1 className="text-xl font-semibold">Văn phòng ảo</h1>
        {error && <ErrorBanner message={error} />}
        <Loading />
      </section>
    );
  }

  if (snapshot.workers.length === 0) {
    return (
      <section className="space-y-4">
        <h1 className="text-xl font-semibold">Văn phòng ảo</h1>
        {error && <ErrorBanner message={error} />}
        <Empty
          title="Chưa có máy nào kết nối"
          action={
            <Link
              to="/my/machines"
              className="rounded-md bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700"
            >
              Thêm máy
            </Link>
          }
        />
      </section>
    );
  }

  const busyJobs = officeState
    ? Object.values(officeState.chars).filter((c) => c.task).length
    : 0;
  const breakAgents = officeState
    ? Object.values(officeState.chars).filter((c) => c.place === 'break').length
    : 0;
  const needsAuthCount = officeState
    ? Object.values(officeState.chars).filter((c) => c.place === 'auth').length
    : 0;
  const usersList = officeState ? Object.values(officeState.users) : [];
  const totalUsers = usersList.length;
  const onlineUsers = usersList.filter((u) => u.online).length;

  const leadDisplay = officeState?.lead
    ? officeState.lead
    : officeState?.vacantNote
      ? `Ghế trống (${officeState.vacantNote})`
      : 'Ghế trống';

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Văn phòng ảo</h1>
      </div>

      {error && <ErrorBanner message={error} />}

      {/* Canvas container with overlay */}
      <div className="overflow-x-auto max-w-full">
        <div className="relative inline-block overflow-hidden rounded-xl border border-slate-700 bg-slate-900 shadow-lg">
        <canvas
          ref={canvasRef}
          data-testid="office-canvas"
          width={layout?.width ?? 480}
          height={layout?.height ?? 260}
          onMouseMove={handleMouseMove}
          onMouseLeave={handleMouseLeave}
          className="block max-w-full rounded"
          style={{
            imageRendering: 'pixelated',
            width: (layout?.width ?? 480) * 2,
            height: (layout?.height ?? 260) * 2,
          }}
        />

        {/* DOM overlay layer */}
        {layout && officeState && (
          <div className="pointer-events-none absolute inset-0 overflow-hidden">
            {/* INOVA Logo on back wall */}
            <div
              style={{
                position: 'absolute',
                left: `${(layout.zones.logo.x / layout.width) * 100}%`,
                top: `${(layout.zones.logo.y / layout.height) * 100}%`,
                width: `${(layout.zones.logo.w / layout.width) * 100}%`,
                height: `${(layout.zones.logo.h / layout.height) * 100}%`,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              {logoError ? (
                <span
                  data-testid="logo-fallback"
                  className="font-bold tracking-wider"
                  style={{ color: '#c8102e', fontSize: '18px' }}
                >
                  INOVA
                </span>
              ) : (
                <img
                  alt="INOVA"
                  src="/brand/inova-logo.png"
                  onError={() => setLogoError(true)}
                  style={{ maxHeight: '100%', maxWidth: '100%', objectFit: 'contain' }}
                />
              )}
            </div>

            {/* INOVA Icon at command door */}
            <img
              data-testid="inova-icon-door"
              src="/brand/inova-icon.png"
              alt=""
              style={{
                position: 'absolute',
                left: `${(layout.zones.commandDoor.x / layout.width) * 100}%`,
                top: `${(layout.zones.commandDoor.y / layout.height) * 100}%`,
                width: '16px',
                height: '16px',
                objectFit: 'contain',
              }}
            />

            {/* INOVA Icon at lobby rug */}
            <img
              data-testid="inova-icon-rug"
              src="/brand/inova-icon.png"
              alt=""
              style={{
                position: 'absolute',
                left: `${((layout.zones.lobbyRug.x + Math.floor(layout.zones.lobbyRug.w / 2) - 8) / layout.width) * 100}%`,
                top: `${((layout.zones.lobbyRug.y + Math.floor(layout.zones.lobbyRug.h / 2) - 8) / layout.height) * 100}%`,
                width: '16px',
                height: '16px',
                objectFit: 'contain',
              }}
            />

            {/* Seats */}
            {layout.seats.map((seat) => (
              <div
                key={seat.index}
                data-testid={`seat-${seat.index}`}
                data-agent={seat.agentId ?? undefined}
                data-empty={seat.agentId ? undefined : 'true'}
                style={{
                  position: 'absolute',
                  left: `${(seat.x / layout.width) * 100}%`,
                  top: `${(seat.y / layout.height) * 100}%`,
                  width: `${(seat.w / layout.width) * 100}%`,
                  height: `${(seat.h / layout.height) * 100}%`,
                }}
              />
            ))}

            {/* Machine labels */}
            {layout.machineLabels.map((ml) => (
              <div
                key={ml.workerId}
                data-testid={`machine-label-${ml.workerId}`}
                style={{
                  position: 'absolute',
                  left: `${(ml.x / layout.width) * 100}%`,
                  top: `${(ml.y / layout.height) * 100}%`,
                  fontSize: '11px',
                  lineHeight: '14px',
                  fontWeight: 600,
                  color: '#f8fafc',
                  background: 'rgba(30, 27, 46, 0.78)',
                  padding: '0 6px',
                  borderRadius: '4px',
                  whiteSpace: 'nowrap',
                  pointerEvents: 'none',
                }}
              >
                {ml.workerId}
                {officeState.ghNeeds.has(ml.workerId) && ' 🔑'}
              </div>
            ))}

            {/* User desks */}
            {layout.userDesks.map((ud) => {
              const userState = officeState.users[ud.login];
              const isOnline = userState ? userState.online : ud.online;
              return (
                <div
                  key={ud.login}
                  data-testid={`user-${ud.login}`}
                  data-online={String(isOnline)}
                  style={{
                    position: 'absolute',
                    left: `${(ud.x / layout.width) * 100}%`,
                    top: `${(ud.y / layout.height) * 100}%`,
                    fontSize: '11px',
                    lineHeight: '14px',
                    fontWeight: 600,
                    color: isOnline ? '#f8fafc' : '#cbd5e1',
                    background: isOnline ? 'rgba(30, 27, 46, 0.85)' : 'rgba(30, 27, 46, 0.55)',
                    padding: '0 6px',
                    borderRadius: '4px',
                    whiteSpace: 'nowrap',
                    pointerEvents: 'none',
                  }}
                >
                  <span>{ud.login}</span> {isOnline ? '🟢' : '· vắng'}
                </div>
              );
            })}

            {/* Characters for agents */}
            {Object.entries(officeState.chars).map(([agentId, char]) => (
              <div
                key={agentId}
                data-testid={`char-${agentId}`}
                data-place={char.place}
                style={{
                  position: 'absolute',
                  opacity: 0,
                  pointerEvents: 'none',
                }}
              >
                <span>{agentId}</span>
                <span>{char.place}</span>
                {char.place === 'auth' && ' 🔑'}
              </div>
            ))}
          </div>
        )}

        {/* Floating tooltip */}
        {hoveredInfo && (
          <div
            className="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full rounded bg-slate-950/90 px-2 py-1 text-xs text-white shadow"
            style={{ left: hoveredInfo.x, top: hoveredInfo.y - 8 }}
          >
            {hoveredInfo.text}
          </div>
        )}
      </div>
      </div>

      {/* Status bar */}
      <div
        data-testid="office-status"
        className="flex flex-wrap items-center justify-between gap-4 rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm text-slate-700 shadow-sm"
      >
        <div className="flex flex-wrap items-center gap-4">
          <div>
            <span className="text-slate-500">Lead: </span>
            <span className="font-medium text-slate-900">{leadDisplay}</span>
          </div>
          <div>
            <span className="text-slate-500">Đang chạy </span>
            <span className="font-semibold text-emerald-600">{busyJobs}</span>
          </div>
          <div>
            <span className="text-slate-500">Nghỉ </span>
            <span className="font-semibold text-amber-600">{breakAgents}</span>
          </div>
          <div>
            <span className="text-slate-500">Cần đăng nhập </span>
            <span className="font-semibold text-rose-600">{needsAuthCount}</span>
          </div>
          <div>
            <span className="text-slate-500">Online </span>
            <span className="font-semibold text-blue-600">
              {onlineUsers}/{totalUsers}
            </span>
          </div>
        </div>

        <button
          onClick={() => setIsPaused((p) => !p)}
          className="rounded border border-slate-300 px-3 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50"
        >
          {isPaused ? '▶ Tiếp tục hoạt cảnh' : '⏸ Tạm dừng hoạt cảnh'}
        </button>
      </div>
    </section>
  );
}
