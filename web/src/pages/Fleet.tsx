import { useEffect, useState, useCallback } from 'react';
import { Link } from 'react-router';
import { api, ApiError } from '../api/client';
import { Empty, ErrorBanner, Loading } from '../components/StateView';

export interface LeadState {
  state: 'HELD' | 'VACANT';
  agent_id: string | null;
  epoch: number;
  expires_at: string | null;
  vacant_since: string | null;
  eta: string | null;
  waiting_jobs: number;
}

export interface Agent {
  id: string;
  kind: 'CLAUDE' | 'ANTIGRAVITY';
  status: 'IDLE' | 'BUSY' | 'COOLDOWN';
  cooldown_until: string | null;
  lead_priority: number | null;
  soft_max_jobs: number | null;
  soft_window_hours: number;
  jobs_in_window: number;
  current_job: {
    job_id: string;
    task_id: string;
    kind: string;
    attempt: number;
  } | null;
}

export interface Worker {
  id: string;
  name: string;
  os: string;
  conn_status: 'ONLINE' | 'OFFLINE';
  offline_since: string | null;
  agents: Agent[];
}

export interface FleetData {
  lead: LeadState;
  workers: Worker[];
}

function LedDot({ status }: { status: 'ONLINE' | 'OFFLINE' | 'IDLE' | 'BUSY' | 'COOLDOWN' }) {
  let color = 'var(--color-led-offline)';
  if (status === 'ONLINE' || status === 'IDLE') color = 'var(--color-led-online)';
  else if (status === 'BUSY') color = 'var(--color-led-busy)';
  else if (status === 'COOLDOWN') color = 'var(--color-led-cooldown)';

  return (
    <span
      className="inline-block h-2.5 w-2.5 rounded-full"
      style={{ backgroundColor: color }}
      aria-hidden="true"
    />
  );
}

export default function Fleet() {
  const [data, setData] = useState<FleetData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const fetchFleet = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.get<FleetData>('/api/fleet');
      setData(res);
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(err.message || err.code);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Không thể kết nối đến máy chủ Fleet');
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchFleet();
  }, [fetchFleet]);

  const handleRevoke = async () => {
    if (!window.confirm('Thu hồi vai lead hiện tại?')) {
      return;
    }
    setActionError(null);
    try {
      await api.post('/api/admin/lead/revoke');
      await fetchFleet();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setActionError(err.message || err.code);
      } else if (err instanceof Error) {
        setActionError(err.message);
      } else {
        setActionError('Lỗi khi thu hồi vai');
      }
    }
  };

  const handleAssign = async (agentId: string) => {
    if (!window.confirm(`Trao vai lead cho ${agentId}?`)) {
      return;
    }
    setActionError(null);
    try {
      await api.post('/api/admin/lead/assign', { agent_id: agentId });
      await fetchFleet();
    } catch (err: unknown) {
      if (err instanceof ApiError && (err.code === 'AGENT_NOT_ELIGIBLE' || err.status === 409)) {
        setActionError('Agent không còn đủ điều kiện nhận vai (đang bận/nghỉ/mất kết nối)');
      } else if (err instanceof ApiError) {
        setActionError(err.message || err.code);
      } else if (err instanceof Error) {
        setActionError(err.message);
      } else {
        setActionError('Lỗi khi trao vai');
      }
    }
  };

  if (loading) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Quản trị cụm máy (Fleet)</h1>
        <Loading rows={4} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Quản trị cụm máy (Fleet)</h1>
        <ErrorBanner message={error} onRetry={fetchFleet} />
      </div>
    );
  }

  if (!data?.workers || data.workers.length === 0) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">Quản trị cụm máy (Fleet)</h1>
        </div>
        <Empty
          title="Chưa có máy nào kết nối"
          action={
            <button
              type="button"
              className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800"
            >
              Thêm máy mới
            </button>
          }
        />
      </div>
    );
  }

  const { lead, workers } = data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">Quản trị cụm máy (Fleet)</h1>
          <p className="mt-1 text-sm text-slate-500">
            Giám sát tình trạng runner máy con, điều phối agent và luân chuyển vai Trưởng phòng.
          </p>
        </div>
      </div>

      {actionError && <ErrorBanner message={actionError} />}

      {lead.state === 'VACANT' ? (
        <div
          data-testid="lead-card"
          className="rounded-xl border border-amber-300 bg-amber-50 p-5 shadow-xs"
        >
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-start gap-3">
              <span className="text-2xl" aria-hidden="true">⚠️</span>
              <div>
                <h2 className="text-base font-semibold text-amber-900">Chưa có trưởng phòng</h2>
                <p className="mt-0.5 text-sm text-amber-700">
                  Hệ thống đang chạy ở chế độ trống lead (epoch {lead.epoch}). Việc trí tuệ đang xếp hàng.
                </p>
              </div>
            </div>
            <div className="text-left sm:text-right">
              <div className="text-sm font-medium text-amber-900">
                Việc chờ: <span className="font-bold">{lead.waiting_jobs}</span>
              </div>
              {lead.eta && (
                <div className="mt-0.5 text-xs text-amber-700">
                  Dự kiến có lead: <span className="font-semibold">{lead.eta}</span>
                </div>
              )}
            </div>
          </div>
        </div>
      ) : (
        <div
          data-testid="lead-card"
          className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm"
        >
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <div className="flex items-center gap-2">
                <span className="text-lg" aria-hidden="true">👑</span>
                <h2 className="text-base font-semibold text-slate-900">
                  Trưởng phòng hiện tại: {lead.agent_id}
                </h2>
              </div>
              <p className="mt-1 text-sm text-slate-500">
                Giữ quyền điều phối · <span className="font-mono font-medium text-slate-700">epoch {lead.epoch}</span>
                {lead.expires_at && ` · Hết hạn: ${lead.expires_at}`}
              </p>
            </div>
            <div>
              <button
                type="button"
                onClick={handleRevoke}
                className="rounded-lg border border-red-200 bg-white px-3 py-1.5 text-sm font-medium text-red-600 shadow-xs hover:bg-red-50 hover:border-red-300"
              >
                Thu hồi vai
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="space-y-4">
        {workers.map((worker) => (
          <div
            key={worker.id}
            data-testid={`worker-${worker.id}`}
            className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm"
          >
            <div className="flex items-center justify-between border-b border-slate-100 pb-3">
              <div className="flex items-center gap-3">
                <LedDot status={worker.conn_status} />
                <span className="font-semibold text-slate-900">{worker.name}</span>
                <span className="rounded bg-slate-100 px-2 py-0.5 font-mono text-xs text-slate-600 uppercase">
                  {worker.os}
                </span>
              </div>
              <div className="flex items-center gap-3">
                <Link
                  to={`/fleet/${worker.id}/terminal`}
                  className="rounded border border-slate-200 bg-white px-2.5 py-1 text-xs font-medium text-indigo-600 shadow-xs hover:bg-slate-50 hover:text-indigo-700"
                >
                  Xem log
                </Link>
                <span
                  className={`rounded px-2.5 py-0.5 text-xs font-semibold ${
                    worker.conn_status === 'ONLINE'
                      ? 'bg-emerald-50 text-emerald-700'
                      : 'bg-slate-100 text-slate-600'
                  }`}
                >
                  {worker.conn_status}
                </span>
              </div>
            </div>

            <div className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-2">
              {worker.agents.map((agent) => {
                const isLead = lead.state === 'HELD' && lead.agent_id === agent.id;
                const canAssign =
                  worker.conn_status === 'ONLINE' &&
                  agent.kind === 'CLAUDE' &&
                  agent.status === 'IDLE' &&
                  !isLead;

                return (
                  <div
                    key={agent.id}
                    data-testid={`agent-${agent.id}`}
                    className="flex flex-col justify-between rounded-lg border border-slate-200 bg-slate-50/70 p-3.5"
                  >
                    <div>
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <LedDot status={agent.status} />
                          <span className="font-medium text-slate-800">{agent.id}</span>
                          {isLead && (
                            <span className="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-bold text-amber-800">
                              LEAD
                            </span>
                          )}
                        </div>
                        <span
                          className={`rounded px-2 py-0.5 text-xs font-semibold ${
                            agent.status === 'IDLE'
                              ? 'bg-emerald-100 text-emerald-800'
                              : agent.status === 'BUSY'
                              ? 'bg-amber-100 text-amber-800'
                              : 'bg-purple-100 text-purple-800'
                          }`}
                        >
                          {agent.status}
                        </span>
                      </div>

                      {agent.current_job && (
                        <div className="mt-2 text-xs text-slate-600">
                          Tác vụ hiện tại:{' '}
                          <span className="font-mono font-medium text-slate-900">
                            {agent.current_job.task_id}
                          </span>
                        </div>
                      )}

                      {agent.status === 'COOLDOWN' && agent.cooldown_until && (
                        <div className="mt-2 text-xs text-purple-700">
                          Nghỉ đến: {agent.cooldown_until}
                        </div>
                      )}
                    </div>

                    {canAssign && (
                      <div className="mt-3 flex justify-end">
                        <button
                          type="button"
                          onClick={() => handleAssign(agent.id)}
                          className="rounded bg-indigo-600 px-3 py-1 text-xs font-medium text-white shadow-xs hover:bg-indigo-700"
                        >
                          Trao vai
                        </button>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
