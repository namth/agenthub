import { useState, useEffect, useCallback } from 'react';
import { api, ApiError } from '../api/client';
import { Empty, ErrorBanner, Loading } from '../components/StateView';
import AddMachineDialog from '../components/AddMachineDialog';

export interface AgentJob {
  job_id: string;
  task_id: string;
  kind: string;
  attempt: number;
}

export interface MachineAgent {
  id: string;
  kind: string;
  status: string;
  projects?: string[];
  current_job?: AgentJob | null;
}

export interface Machine {
  id: string;
  name: string;
  os: string;
  conn_status: 'ONLINE' | 'OFFLINE';
  version?: string;
  paused: boolean;
  agents: MachineAgent[];
}

export interface Project {
  id: string;
  name: string;
  github_repo: string;
  status: string;
  role: string;
}

function getCommonProject(agents: MachineAgent[]): string {
  if (!agents || agents.length === 0) return '';
  const firstProjects = agents[0].projects ?? [];
  if (firstProjects.length !== 1) return '';
  const target = firstProjects[0];
  const allMatch = agents.every((a) => {
    const p = a.projects ?? [];
    return p.length === 1 && p[0] === target;
  });
  return allMatch ? target : '';
}

export default function MyMachines() {
  const [machines, setMachines] = useState<Machine[] | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  const fetchData = useCallback(async () => {
    setError(null);
    try {
      const [mList, pList] = await Promise.all([
        api.get<Machine[]>('/api/my/machines'),
        api.get<Project[]>('/api/projects').catch(() => [] as Project[]),
      ]);
      setMachines(mList || []);
      setProjects(pList || []);
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(err.message || err.code);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Lỗi khi tải danh sách máy');
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const handleTogglePause = async (m: Machine) => {
    setActionError(null);
    try {
      await api.patch(`/api/my/machines/${m.id}`, { paused: !m.paused });
      await fetchData();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setActionError(err.message || err.code);
      } else if (err instanceof Error) {
        setActionError(err.message);
      } else {
        setActionError('Lỗi khi cập nhật trạng thái tạm dừng');
      }
    }
  };

  const handleDelete = async (m: Machine) => {
    if (!window.confirm(`Gỡ máy ${m.name}?`)) {
      return;
    }
    setActionError(null);
    try {
      await api.del(`/api/my/machines/${m.id}`);
      await fetchData();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setActionError(err.message || err.code);
      } else if (err instanceof Error) {
        setActionError(err.message);
      } else {
        setActionError('Lỗi khi gỡ máy');
      }
    }
  };

  const handleProjectChange = async (m: Machine, projectId: string) => {
    setActionError(null);
    try {
      const body = projectId === '' ? { projects: null } : { projects: [projectId] };
      await api.patch(`/api/my/machines/${m.id}`, body);
      await fetchData();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setActionError(err.message || err.code);
      } else if (err instanceof Error) {
        setActionError(err.message);
      } else {
        setActionError('Lỗi khi cập nhật dự án nhận việc');
      }
    }
  };

  if (loading && machines === null) {
    return (
      <section className="space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">Máy của tôi</h1>
        </div>
        <Loading rows={4} />
      </section>
    );
  }

  if (error && machines === null) {
    return (
      <section className="space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">Máy của tôi</h1>
        </div>
        <ErrorBanner message={error} onRetry={fetchData} />
      </section>
    );
  }

  if (machines && machines.length === 0) {
    return (
      <section className="space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">Máy của tôi</h1>
          <button
            type="button"
            onClick={() => setDialogOpen(true)}
            className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-xs hover:bg-slate-800"
          >
            Thêm máy mới
          </button>
        </div>
        {actionError && <ErrorBanner message={actionError} />}
        <Empty
          title="Chưa có máy nào"
          action={
            <button
              type="button"
              onClick={() => setDialogOpen(true)}
              className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-xs hover:bg-slate-800"
            >
              Thêm máy mới
            </button>
          }
        />
        <AddMachineDialog
          open={dialogOpen}
          onClose={() => {
            setDialogOpen(false);
            fetchData();
          }}
        />
      </section>
    );
  }

  return (
    <section className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">Máy của tôi</h1>
          <p className="mt-1 text-sm text-slate-500">
            Quản lý máy con cá nhân kết nối vào hệ thống, chỉ định dự án nhận việc.
          </p>
        </div>
        <button
          type="button"
          onClick={() => setDialogOpen(true)}
          className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-xs hover:bg-slate-800"
        >
          Thêm máy mới
        </button>
      </div>

      {actionError && <ErrorBanner message={actionError} />}

      <div className="space-y-4">
        {machines?.map((m) => (
          <div
            key={m.id}
            data-testid={`machine-${m.id}`}
            className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm space-y-4"
          >
            <div className="flex items-center justify-between border-b border-slate-100 pb-3">
              <div className="flex items-center gap-3">
                <span className="font-semibold text-slate-900">{m.name}</span>
                <span className="rounded bg-slate-100 px-2 py-0.5 font-mono text-xs uppercase text-slate-600">
                  {m.os}
                </span>
                {m.version && (
                  <span className="font-mono text-xs text-slate-500">
                    {m.version}
                  </span>
                )}
                {m.paused && (
                  <span className="rounded bg-amber-100 px-2 py-0.5 text-xs font-semibold text-amber-800">
                    Tạm dừng
                  </span>
                )}
              </div>
              <div className="flex items-center gap-3">
                <span
                  className={`rounded px-2.5 py-0.5 text-xs font-semibold ${
                    m.conn_status === 'ONLINE'
                      ? 'bg-emerald-50 text-emerald-700'
                      : 'bg-slate-100 text-slate-600'
                  }`}
                >
                  {m.conn_status}
                </span>
                <button
                  type="button"
                  onClick={() => handleTogglePause(m)}
                  className="rounded border border-slate-200 bg-white px-2.5 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50"
                >
                  {m.paused ? 'Tiếp tục' : 'Tạm dừng'}
                </button>
                <button
                  type="button"
                  onClick={() => handleDelete(m)}
                  className="rounded border border-red-200 bg-white px-2.5 py-1 text-xs font-medium text-red-600 hover:bg-red-50 hover:border-red-300"
                >
                  Gỡ
                </button>
              </div>
            </div>

            {/* Select Project */}
            <div className="flex items-center gap-2">
              <label htmlFor={`proj-${m.id}`} className="text-xs font-medium text-slate-600">
                Dự án nhận việc
              </label>
              <select
                id={`proj-${m.id}`}
                aria-label="Dự án nhận việc"
                value={getCommonProject(m.agents)}
                onChange={(e) => handleProjectChange(m, e.target.value)}
                className="rounded-md border border-slate-300 bg-white px-2.5 py-1 text-xs text-slate-700 shadow-xs focus:border-indigo-500 focus:outline-none"
              >
                <option value="">Mọi dự án</option>
                {projects.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.github_repo}
                  </option>
                ))}
              </select>
            </div>

            {/* Agents */}
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              {m.agents.map((agent) => (
                <div
                  key={agent.id}
                  className="rounded-lg border border-slate-200 bg-slate-50/70 p-3 text-xs space-y-1.5"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-medium text-slate-800">{agent.id}</span>
                    <div className="flex items-center gap-1.5">
                      <span className="rounded bg-slate-200 px-1.5 py-0.5 text-[10px] font-mono text-slate-700">
                        {agent.kind}
                      </span>
                      <span className="font-semibold text-slate-600">{agent.status}</span>
                    </div>
                  </div>
                  {agent.current_job && (
                    <div className="text-slate-600">
                      Tác vụ hiện tại:{' '}
                      <span className="font-mono font-medium text-slate-900">
                        {agent.current_job.task_id}
                      </span>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>

      <AddMachineDialog
        open={dialogOpen}
        onClose={() => {
          setDialogOpen(false);
          fetchData();
        }}
      />
    </section>
  );
}
