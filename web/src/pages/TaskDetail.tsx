import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'react-router';
import { api, ApiError } from '../api/client';
import { Empty, ErrorBanner, Loading } from '../components/StateView';
import type { Role } from '../nav';

export interface Subtask {
  id: string;
  title: string;
  status: string;
  github_issue_id?: number | null;
  pr_url?: string | null;
}

export interface TaskDetailData {
  id: string;
  title: string;
  description: string;
  status: string;
  github_issue_id?: number | null;
  pr_url?: string | null;
  parent_id?: string | null;
  created_at?: string;
  subtasks?: Subtask[];
}

export interface TaskDetailProps {
  role?: Role;
}

export const STATUS_LABELS: Record<string, string> = {
  PENDING_APPROVAL: 'Chờ duyệt',
  PLANNING: 'Lập kế hoạch',
  READY: 'Sẵn sàng',
  IN_PROGRESS: 'Đang làm',
  WAITING_QA: 'Chờ kiểm định',
  QA_PASSED: 'Kiểm định đạt',
  DONE: 'Hoàn thành',
  REJECTED: 'Bị từ chối',
};

const STATUS_COLORS: Record<string, string> = {
  PENDING_APPROVAL: 'bg-amber-100 text-amber-800 border-amber-200',
  PLANNING: 'bg-sky-100 text-sky-800 border-sky-200',
  READY: 'bg-indigo-100 text-indigo-800 border-indigo-200',
  IN_PROGRESS: 'bg-blue-100 text-blue-800 border-blue-200',
  WAITING_QA: 'bg-purple-100 text-purple-800 border-purple-200',
  QA_PASSED: 'bg-emerald-100 text-emerald-800 border-emerald-200',
  DONE: 'bg-emerald-100 text-emerald-800 border-emerald-200',
  REJECTED: 'bg-rose-100 text-rose-800 border-rose-200',
};

export default function TaskDetail({ role = 'PARTNER' }: TaskDetailProps = {}) {
  const { id } = useParams<{ id: string }>();
  const [task, setTask] = useState<TaskDetailData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [acting, setActing] = useState(false);

  const fetchTask = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    setError(null);
    setNotFound(false);
    try {
      const data = await api.get<TaskDetailData>(`/api/tasks/${id}`);
      setTask(data);
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 404) {
        setNotFound(true);
      } else if (err instanceof ApiError) {
        setError(err.message || err.code);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Không thể tải thông tin task');
      }
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchTask();
  }, [fetchTask]);

  const handleApprove = async () => {
    if (!id || acting) return;
    setActing(true);
    setActionError(null);
    try {
      await api.post(`/api/tasks/${id}/approve`);
      await fetchTask();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setActionError(err.message || err.code);
      } else if (err instanceof Error) {
        setActionError(err.message);
      } else {
        setActionError('Không thể duyệt task');
      }
    } finally {
      setActing(false);
    }
  };

  const handleReject = async () => {
    if (!id || acting) return;
    const reason = window.prompt('Lý do từ chối:');
    if (reason === null || reason.trim() === '') {
      return;
    }
    setActing(true);
    setActionError(null);
    try {
      await api.post(`/api/tasks/${id}/reject`, { reason: reason.trim() });
      await fetchTask();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setActionError(err.message || err.code);
      } else if (err instanceof Error) {
        setActionError(err.message);
      } else {
        setActionError('Không thể từ chối task');
      }
    } finally {
      setActing(false);
    }
  };

  if (loading) {
    return (
      <div className="space-y-4">
        <Loading rows={4} />
      </div>
    );
  }

  if (notFound) {
    return (
      <div className="space-y-4">
        <Empty title="Không tìm thấy task" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-4">
        <ErrorBanner message={error} onRetry={fetchTask} />
      </div>
    );
  }

  if (!task) {
    return (
      <div className="space-y-4">
        <Empty title="Không tìm thấy task" />
      </div>
    );
  }

  const canApproveOrReject = role === 'ADMIN' && task.status === 'PENDING_APPROVAL';

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold tracking-tight text-slate-900">{task.title}</h1>
            {task.github_issue_id && (
              <span className="font-mono text-sm text-slate-500">#{task.github_issue_id}</span>
            )}
            <span
              className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${
                STATUS_COLORS[task.status] ?? 'border-slate-200 bg-slate-100 text-slate-700'
              }`}
            >
              {STATUS_LABELS[task.status] ?? task.status}
            </span>
          </div>
        </div>

        {canApproveOrReject && (
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={handleApprove}
              disabled={acting}
              className="rounded-lg bg-green-600 px-4 py-2 text-sm font-medium text-white shadow-xs hover:bg-green-700 disabled:opacity-50"
            >
              Duyệt
            </button>
            <button
              type="button"
              onClick={handleReject}
              disabled={acting}
              className="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white shadow-xs hover:bg-red-700 disabled:opacity-50"
            >
              Từ chối
            </button>
          </div>
        )}
      </div>

      {actionError && <ErrorBanner message={actionError} onRetry={() => setActionError(null)} />}

      <div className="rounded-xl border border-slate-200 bg-white p-6 shadow-xs">
        <h2 className="mb-2 text-sm font-semibold tracking-wider text-slate-500 uppercase">Mô tả chi tiết</h2>
        <p className="text-slate-700 whitespace-pre-wrap">{task.description}</p>
      </div>

      <div className="rounded-xl border border-slate-200 bg-white p-6 shadow-xs">
        <h2 className="mb-4 text-sm font-semibold tracking-wider text-slate-500 uppercase">Danh sách sub-task</h2>
        <div data-testid="subtasks" className="divide-y divide-slate-100">
          {task.subtasks && task.subtasks.length > 0 ? (
            task.subtasks.map((sub) => {
              const prNum = sub.pr_url ? sub.pr_url.split('/').filter(Boolean).pop() : null;
              return (
                <div key={sub.id} className="flex items-center justify-between py-3">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-slate-800">{sub.title}</span>
                    {sub.github_issue_id && (
                      <span className="font-mono text-xs text-slate-400">#{sub.github_issue_id}</span>
                    )}
                  </div>
                  <div className="flex items-center gap-3">
                    <span className="rounded-md bg-slate-100 px-2 py-1 text-xs text-slate-600">
                      {STATUS_LABELS[sub.status] ?? sub.status}
                    </span>
                    {sub.pr_url && (
                      <a
                        href={sub.pr_url}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex items-center text-sm font-medium text-blue-600 hover:text-blue-800 hover:underline"
                      >
                        PR {prNum ? `#${prNum}` : ''}
                      </a>
                    )}
                  </div>
                </div>
              );
            })
          ) : (
            <p className="py-4 text-sm text-slate-500">Chưa có sub-task nào</p>
          )}
        </div>
      </div>
    </div>
  );
}
