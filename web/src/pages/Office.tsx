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
    const es = new EventSource('/api/admin/office/stream');

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
    return computeLayout(snapshot.workers);
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
      // Vẽ ngay một khung: không chờ requestAnimationFrame (bị hoãn khi tab ở nền / vừa mở trang).
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
          (c) => c.task && c.place === 'desk'
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

  // Handle canvas mouse move for tooltips
  const handleMouseMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
    if (!canvasRef.current || !layout || !officeState) return;
    const rect = canvasRef.current.getBoundingClientRect();
    const scaleX = layout.width / rect.width;
    const scaleY = layout.height / rect.height;
    const cx = (e.clientX - rect.left) * scaleX;
    const cy = (e.clientY - rect.top) * scaleY;

    // Check characters
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
      <section className="space-y-4">
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
              to="/fleet"
              className="rounded-md bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700"
            >
              Đăng ký máy
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

      {/* Canvas container with pixelated rendering */}
      <div className="relative inline-block overflow-hidden rounded-xl border border-slate-700 bg-slate-900 p-2 shadow-lg">
        <canvas
          ref={canvasRef}
          data-testid="office-canvas"
          width={layout?.width ?? 384}
          height={layout?.height ?? 272}
          onMouseMove={handleMouseMove}
          onMouseLeave={handleMouseLeave}
          className="block max-w-full rounded"
          style={{
            imageRendering: 'pixelated',
            width: (layout?.width ?? 384) * 2,
            height: (layout?.height ?? 272) * 2,
          }}
        />

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

      {/* Accessible list of agents */}
      {officeState && (
        <div className="sr-only">
          {Object.entries(officeState.chars).map(([agentId, char]) => (
            <div
              key={agentId}
              data-testid={`char-${agentId}`}
              data-place={char.place}
            >
              <span>{agentId}</span>
              <span>{char.place}</span>
              {char.task && <span>{char.task}</span>}
              {char.note && <span>{char.note}</span>}
            </div>
          ))}
        </div>
      )}

      {/* Status bar */}
      <div className="flex flex-wrap items-center justify-between gap-4 rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm text-slate-700 shadow-sm">
        <div className="flex flex-wrap items-center gap-4">
          <div>
            <span className="text-slate-500">Trưởng phòng: </span>
            <span className="font-medium text-slate-900">{leadDisplay}</span>
          </div>
          <div>
            <span className="text-slate-500">Đang chạy: </span>
            <span className="font-semibold text-emerald-600">{busyJobs}</span>
            <span className="text-slate-500"> job</span>
          </div>
          <div>
            <span className="text-slate-500">Phòng nghỉ: </span>
            <span className="font-semibold text-amber-600">{breakAgents}</span>
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
