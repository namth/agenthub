import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useParams } from 'react-router';
import { Empty, ErrorBanner, Loading } from '../components/StateView';
import { ansiToSegments } from '../components/ansi';

export interface LogLine {
  id: number;
  worker_id?: string;
  assignment_id?: string;
  task_id?: string;
  chunk: string;
  ts?: string;
}

const COLOR_STYLES: Record<string, string> = {
  black: '#64748b',
  red: '#f87171',
  green: '#4ade80',
  yellow: '#facc15',
  blue: '#60a5fa',
  magenta: '#e879f9',
  cyan: '#22d3ee',
  white: '#f8fafc',
};

function AnsiLogLine({ chunk }: { chunk: string }) {
  const segments = useMemo(() => ansiToSegments(chunk), [chunk]);

  return (
    <>
      {segments.map((seg, idx) => {
        if (seg.color) {
          return (
            <span
              key={idx}
              data-color={seg.color}
              className={seg.bold ? 'font-bold' : undefined}
              style={{
                color: COLOR_STYLES[seg.color] ?? seg.color,
                fontWeight: seg.bold ? 700 : undefined,
              }}
            >
              {seg.text}
            </span>
          );
        }
        if (seg.bold) {
          return (
            <span key={idx} className="font-bold" style={{ fontWeight: 700 }}>
              {seg.text}
            </span>
          );
        }
        return <span key={idx}>{seg.text}</span>;
      })}
    </>
  );
}

export default function LiveTerminal() {
  const { id } = useParams<{ id: string }>();
  const workerId = id ?? '';

  const [logs, setLogs] = useState<LogLine[]>([]);
  const [hasReceivedHistory, setHasReceivedHistory] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedTask, setSelectedTask] = useState<string>('');
  const [pauseScroll, setPauseScroll] = useState(false);

  const terminalRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!workerId) return;

    setHasReceivedHistory(false);
    setLogs([]);
    setError(null);

    const es = new EventSource(`/api/admin/workers/${workerId}/logs/stream`);

    es.onopen = () => {
      setError(null);
    };

    es.onerror = () => {
      setError('Mất kết nối — đang thử lại');
    };

    es.addEventListener('history', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data);
        if (Array.isArray(data)) {
          setLogs(data.slice(-5000));
        }
      } catch {
        // ignore JSON parse error
      }
      setHasReceivedHistory(true);
    });

    es.addEventListener('log', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data);
        if (data) {
          setLogs((prev) => {
            const next = [...prev, data];
            return next.length > 5000 ? next.slice(-5000) : next;
          });
        }
      } catch {
        // ignore JSON parse error
      }
    });

    return () => {
      es.close();
    };
  }, [workerId]);

  // Thu thập danh sách các task_id đã thấy
  const taskIds = useMemo(() => {
    const set = new Set<string>();
    for (const log of logs) {
      if (log.task_id) {
        set.add(log.task_id);
      }
    }
    return Array.from(set);
  }, [logs]);

  // Lọc danh sách log theo task đã chọn
  const filteredLogs = useMemo(() => {
    if (!selectedTask) return logs;
    return logs.filter((log) => log.task_id === selectedTask);
  }, [logs, selectedTask]);

  // Tự động cuộn xuống cuối khi có log mới
  useEffect(() => {
    if (!pauseScroll && terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
      bottomRef.current?.scrollIntoView?.({ behavior: 'smooth' });
    }
  }, [filteredLogs, pauseScroll]);

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-bold tracking-tight text-slate-900">
              Live terminal: {workerId}
            </h1>
            <span className="inline-flex items-center rounded-full bg-emerald-50 px-2.5 py-0.5 text-xs font-medium text-emerald-700">
              SSE Stream
            </span>
          </div>
          <p className="mt-1 text-sm text-slate-500">
            Xem log thời gian thực từ máy con (tối đa 5.000 dòng).
          </p>
        </div>
        <Link
          to="/fleet"
          className="self-start rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 shadow-xs hover:bg-slate-50 sm:self-auto"
        >
          ← Quay lại cụm máy
        </Link>
      </div>

      {error && <ErrorBanner message={error} />}

      {!hasReceivedHistory ? (
        <Loading rows={8} />
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-4 rounded-lg border border-slate-200 bg-white p-3 shadow-xs">
            <div className="flex items-center gap-2">
              <label htmlFor="task-filter" className="text-sm font-medium text-slate-700">
                Lọc theo task
              </label>
              <select
                id="task-filter"
                aria-label="Lọc theo task"
                value={selectedTask}
                onChange={(e) => setSelectedTask(e.target.value)}
                className="rounded-md border border-slate-300 bg-white px-2.5 py-1 text-sm text-slate-800 shadow-xs focus:border-indigo-500 focus:outline-hidden focus:ring-1 focus:ring-indigo-500"
              >
                <option value="">Tất cả</option>
                {taskIds.map((tid) => (
                  <option key={tid} value={tid}>
                    {tid}
                  </option>
                ))}
              </select>
            </div>

            <label className="flex cursor-pointer items-center gap-2 text-sm text-slate-700">
              <input
                type="checkbox"
                checked={pauseScroll}
                onChange={(e) => setPauseScroll(e.target.checked)}
                className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
              />
              <span>Tạm dừng cuộn</span>
            </label>
          </div>

          {logs.length === 0 ? (
            <Empty title="Chưa có log" />
          ) : (
            <div
              ref={terminalRef}
              data-testid="terminal-output"
              className="h-[600px] overflow-y-auto rounded-lg bg-slate-950 p-4 font-mono text-sm text-slate-100 shadow-inner whitespace-pre-wrap break-all"
            >
              {filteredLogs.length === 0 ? (
                <div className="py-12 text-center text-slate-500">Chưa có log cho task này</div>
              ) : (
                filteredLogs.map((log) => (
                  <div key={log.id} data-line={log.id} className="leading-relaxed">
                    <AnsiLogLine chunk={log.chunk} />
                  </div>
                ))
              )}
              <div ref={bottomRef} />
            </div>
          )}
        </>
      )}
    </div>
  );
}
