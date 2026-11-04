import { useEffect, useState, useCallback } from 'react';
import { Link } from 'react-router';
import { api, ApiError } from '../api/client';
import { Empty, ErrorBanner, Loading } from '../components/StateView';
import type { Role } from '../nav';
import { ALL_PROJECTS, type Project, useSelectedProject } from '../project';

export interface Task {
  id: string;
  project_id?: string | null;
  title: string;
  status: string;
  github_issue_id?: number | null;
  parent_id?: string | null;
  created_at?: string;
}

export interface Column {
  key: string;
  title: string;
  tasks: Task[];
}

export interface KanbanBoard {
  columns: Column[];
}

export default function Kanban({ role = 'PARTNER' }: { role?: Role }) {
  const selectedProjectId = useSelectedProject();
  const [board, setBoard] = useState<KanbanBoard | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [approvingId, setApprovingId] = useState<string | null>(null);

  const fetchProjects = useCallback(async () => {
    try {
      const data = await api.get<Project[]>('/api/projects');
      setProjects(data || []);
    } catch {
      // Bỏ qua lỗi theo đặc tả (không hiện banner)
    }
  }, []);

  useEffect(() => {
    fetchProjects();
  }, [fetchProjects, selectedProjectId]);

  const fetchTasks = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const url =
        selectedProjectId && selectedProjectId !== ALL_PROJECTS
          ? `/api/tasks?project_id=${encodeURIComponent(selectedProjectId)}`
          : '/api/tasks';
      const data = await api.get<KanbanBoard>(url);
      setBoard(data);
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(err.message || err.code);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Không thể tải danh sách tác vụ');
      }
    } finally {
      setLoading(false);
    }
  }, [selectedProjectId]);

  useEffect(() => {
    fetchTasks();
  }, [fetchTasks]);

  const handleApprove = async (taskId: string) => {
    setApprovingId(taskId);
    setActionError(null);
    try {
      await api.post(`/api/tasks/${taskId}/approve`);
      await fetchTasks();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setActionError(err.message || err.code);
      } else if (err instanceof Error) {
        setActionError(err.message);
      } else {
        setActionError('Lỗi khi phê duyệt tác vụ');
      }
    } finally {
      setApprovingId(null);
    }
  };

  const currentProject = projects.find((p) => p.id === selectedProjectId);
  const isArchived = currentProject?.status === 'ARCHIVED';
  const canApprove = !isArchived && (role === 'ADMIN' || currentProject?.role === 'ADMIN');

  if (loading) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Bảng điều phối Kanban</h1>
        <Loading rows={4} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Bảng điều phối Kanban</h1>
        <ErrorBanner message={error} onRetry={fetchTasks} />
      </div>
    );
  }

  const columns = board?.columns ?? [];
  const totalTasks = columns.reduce((acc, col) => acc + (col.tasks?.length ?? 0), 0);

  if (totalTasks === 0) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">Bảng điều phối Kanban</h1>
          <Link
            to="/tasks/new"
            className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800"
          >
            Tạo yêu cầu
          </Link>
        </div>
        {isArchived && (
          <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
            Dự án đã lưu trữ — chỉ xem
          </div>
        )}
        <Empty
          title="Chưa có tác vụ nào trong hàng đợi"
          action={
            <Link
              to="/tasks/new"
              className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-800"
            >
              Tạo yêu cầu đầu tiên
            </Link>
          }
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">Bảng điều phối Kanban</h1>
          <p className="mt-1 text-sm text-slate-500">
            Theo dõi và điều phối luồng công việc giữa Admin, Partner và AI Agent.
          </p>
        </div>
        <Link
          to="/tasks/new"
          className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800"
        >
          Tạo yêu cầu
        </Link>
      </div>

      {isArchived && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          Dự án đã lưu trữ — chỉ xem
        </div>
      )}

      {actionError && <ErrorBanner message={actionError} />}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-5">
        {columns.map((column) => (
          <div
            key={column.key}
            data-testid={`column-${column.key}`}
            className="flex flex-col rounded-xl border border-slate-200 bg-slate-100/70 p-3 shadow-xs"
          >
            <div className="mb-3 flex items-center justify-between px-1">
              <h2 className="text-sm font-semibold text-slate-700">{column.title}</h2>
              <span className="rounded-full bg-slate-200 px-2 py-0.5 text-xs font-medium text-slate-600">
                {column.tasks?.length ?? 0}
              </span>
            </div>

            <div className="flex flex-1 flex-col gap-2">
              {column.tasks?.map((task) => (
                <div
                  key={task.id}
                  className="rounded-lg border border-slate-200 bg-white p-3 shadow-xs transition hover:shadow-sm"
                >
                  <Link
                    to={`/tasks/${task.id}`}
                    className="block font-medium text-slate-900 hover:text-blue-600"
                  >
                    {task.title}
                  </Link>

                  {task.github_issue_id && (
                    <div className="mt-1 text-xs text-slate-500">
                      Issue #{task.github_issue_id}
                    </div>
                  )}

                  {selectedProjectId === ALL_PROJECTS && task.project_id && (
                    <div className="mt-1 text-xs font-medium text-slate-600">
                      {projects.find((p) => p.id === task.project_id)?.github_repo || task.project_id}
                    </div>
                  )}

                  {column.key === 'backlog' && canApprove && (
                    <div className="mt-3 flex justify-end">
                      <button
                        type="button"
                        disabled={approvingId === task.id}
                        onClick={() => handleApprove(task.id)}
                        className="rounded bg-emerald-600 px-2.5 py-1 text-xs font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
                      >
                        {approvingId === task.id ? 'Đang duyệt...' : 'Duyệt'}
                      </button>
                    </div>
                  )}
                </div>
              ))}

              {(!column.tasks || column.tasks.length === 0) && (
                <div className="flex flex-1 items-center justify-center rounded-lg border border-dashed border-slate-300 py-8 text-center text-xs text-slate-400">
                  Trống
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
