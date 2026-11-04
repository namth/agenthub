import { useEffect, useState, useCallback } from 'react';
import { api, ApiError } from '../api/client';
import { ErrorBanner, Loading } from '../components/StateView';
import type { Project } from '../project';

export interface ProjectWithOps extends Project {
  allowed_commands?: string[];
  needs_ci_for_integration?: boolean;
  is_sandbox?: boolean;
}

export interface GitHubRepo {
  full_name: string;
  default_branch: string;
  private: boolean;
  added: boolean;
}

interface SmokeStep {
  step: string;
  agent_id: string;
  started_at: string | null;
  ended_at: string | null;
  ok: boolean | null;
  note: string;
}

interface SmokeRun {
  id: string;
  project_id: string;
  task_id: string;
  status: string;
  steps: SmokeStep[];
  started_at: string;
  ended_at: string | null;
}

function ProjectOps({
  project,
  onReload,
}: {
  project: ProjectWithOps;
  onReload: () => Promise<void>;
}) {
  const [commandsText, setCommandsText] = useState(() =>
    (project.allowed_commands ?? []).join('\n')
  );
  const [needsCI, setNeedsCI] = useState(() => Boolean(project.needs_ci_for_integration));
  const [isSandbox, setIsSandbox] = useState(() => Boolean(project.is_sandbox));
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const [runs, setRuns] = useState<SmokeRun[]>([]);
  const [startingSmoke, setStartingSmoke] = useState(false);

  const fetchRuns = useCallback(async () => {
    if (!isSandbox) return;
    try {
      const data = await api.get<SmokeRun[]>(`/api/admin/projects/${project.id}/smoke-runs`);
      setRuns(data || []);
    } catch {
      // Bỏ qua lỗi tải lịch sử chạy thử
    }
  }, [project.id, isSandbox]);

  useEffect(() => {
    if (isSandbox) {
      fetchRuns();
    }
  }, [fetchRuns, isSandbox]);

  const hasRunning = runs.some((r) => r.status === 'RUNNING');

  useEffect(() => {
    if (!isSandbox || !hasRunning) return;
    const interval = setInterval(() => {
      fetchRuns();
    }, 3000);
    return () => clearInterval(interval);
  }, [isSandbox, hasRunning, fetchRuns]);

  const handleSaveOps = async () => {
    setSaveError(null);
    setSaving(true);
    const allowed = commandsText
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
    try {
      await api.patch(`/api/admin/projects/${project.id}`, {
        allowed_commands: allowed,
        needs_ci_for_integration: needsCI,
        is_sandbox: isSandbox,
      });
      await onReload();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setSaveError(err.message || err.code);
      } else if (err instanceof Error) {
        setSaveError(err.message);
      } else {
        setSaveError('Lỗi khi lưu cấu hình');
      }
    } finally {
      setSaving(false);
    }
  };

  const handleStartSmoke = async () => {
    setStartingSmoke(true);
    try {
      await api.post(`/api/admin/projects/${project.id}/smoke-run`);
      await fetchRuns();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setSaveError(err.message || err.code);
      } else if (err instanceof Error) {
        setSaveError(err.message);
      }
    } finally {
      setStartingSmoke(false);
    }
  };

  return (
    <div
      data-testid={`project-ops-${project.id}`}
      className="rounded-xl border border-slate-200 bg-white p-5 shadow-xs space-y-4"
    >
      <div className="flex items-center justify-between">
        <h3 className="font-semibold text-slate-900 text-sm">
          Cấu hình vận hành: {project.github_repo || project.name}
        </h3>
        {isSandbox && (
          <span className="rounded bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-700">
            Sandbox
          </span>
        )}
      </div>

      <div className="space-y-2">
        <label className="block text-xs font-medium text-slate-700">
          Lệnh được phép (mỗi dòng một lệnh):
        </label>
        <textarea
          aria-label="Lệnh được phép"
          value={commandsText}
          onChange={(e) => {
            setCommandsText(e.target.value);
            if (saveError) setSaveError(null);
          }}
          rows={3}
          className="w-full rounded border border-slate-300 p-2 text-xs font-mono"
          placeholder="Ví dụ: go test&#10;gofmt&#10;npx vitest"
        />
        <div className="text-xs text-slate-500">
          Mặc định: git, gh pr create, gh pr view, ls, cat, grep, find, head, tail, wc
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-6 text-xs text-slate-700">
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            aria-label="Cần CI cho test tích hợp"
            checked={needsCI}
            onChange={(e) => {
              setNeedsCI(e.target.checked);
              if (saveError) setSaveError(null);
            }}
            className="rounded border-slate-300 text-slate-900"
          />
          <span>Cần CI cho test tích hợp</span>
        </label>

        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            aria-label="Dự án sandbox"
            checked={isSandbox}
            onChange={(e) => {
              setIsSandbox(e.target.checked);
              if (saveError) setSaveError(null);
            }}
            className="rounded border-slate-300 text-slate-900"
          />
          <span>Dự án sandbox</span>
        </label>
      </div>

      {saveError && (
        <div
          role="alert"
          className="rounded border border-red-200 bg-red-50 p-2 text-xs text-red-700"
        >
          {saveError}
        </div>
      )}

      <div className="flex items-center gap-3 pt-2 border-t border-slate-100">
        <button
          type="button"
          disabled={saving}
          onClick={handleSaveOps}
          className="rounded bg-slate-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-slate-800 disabled:opacity-50"
        >
          Lưu cấu hình
        </button>

        {isSandbox && (
          <button
            type="button"
            disabled={hasRunning || startingSmoke}
            onClick={handleStartSmoke}
            className="rounded bg-emerald-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
          >
            Chạy thử dây chuyền
          </button>
        )}
      </div>

      {isSandbox && (
        <div className="pt-3 border-t border-slate-100 space-y-2">
          <h4 className="text-xs font-semibold text-slate-800">Lần chạy thử</h4>
          {runs.length === 0 ? (
            <div className="text-xs text-slate-500 py-2">Chưa chạy thử lần nào</div>
          ) : (
            <div
              data-testid={`smoke-runs-${project.id}`}
              className="overflow-x-auto rounded border border-slate-200"
            >
              <table className="w-full text-left text-xs">
                <thead className="border-b border-slate-200 bg-slate-50 text-slate-600">
                  <tr>
                    <th className="px-3 py-2">Trạng thái</th>
                    <th className="px-3 py-2">Các bước</th>
                    <th className="px-3 py-2">Thời gian</th>
                    <th className="px-3 py-2">Ghi chú</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-200">
                  {runs.map((r) => (
                    <tr key={r.id}>
                      <td className="px-3 py-2 font-medium">
                        <span
                          className={
                            r.status === 'PASSED'
                              ? 'text-emerald-600'
                              : r.status === 'FAILED'
                              ? 'text-red-600'
                              : r.status === 'TIMEOUT'
                              ? 'text-amber-600'
                              : 'text-blue-600'
                          }
                        >
                          {r.status}
                        </span>
                      </td>
                      <td className="px-3 py-2">
                        <div className="flex flex-wrap gap-2">
                          {r.steps.map((st, i) => (
                            <span
                              key={i}
                              className="rounded bg-slate-100 px-2 py-0.5 text-slate-700"
                            >
                              {st.step}: {st.ok === true ? '✓' : st.ok === false ? '✗' : '…'}
                              {st.agent_id ? ` (${st.agent_id})` : ''}
                              {st.note ? ` [${st.note}]` : ''}
                            </span>
                          ))}
                        </div>
                      </td>
                      <td className="px-3 py-2 text-slate-500">
                        {r.started_at}
                        {r.ended_at ? ` → ${r.ended_at}` : ''}
                      </td>
                      <td className="px-3 py-2 text-slate-600">
                        {r.steps.find((st) => st.note)?.note || '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function ProjectRow({
  project,
  onReload,
}: {
  project: Project;
  onReload: () => Promise<void>;
}) {
  const [weight, setWeight] = useState(String(project.weight ?? 1));
  const [maxParallel, setMaxParallel] = useState(
    project.max_parallel !== null && project.max_parallel !== undefined ? String(project.max_parallel) : ''
  );
  const [rowError, setRowError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [recreatingWebhook, setRecreatingWebhook] = useState(false);

  const handleSave = async () => {
    setRowError(null);
    const trimmedWeight = weight.trim();
    const w = parseInt(trimmedWeight, 10);
    if (isNaN(w) || w < 1 || w > 5 || String(w) !== trimmedWeight) {
      setRowError('Trọng số 1–5');
      return;
    }

    let mp: number | null = null;
    const trimmedMp = maxParallel.trim();
    if (trimmedMp !== '') {
      const p = parseInt(trimmedMp, 10);
      if (isNaN(p) || p < 1 || p > 50 || String(p) !== trimmedMp) {
        setRowError('Song song tối đa 1–50 hoặc để trống');
        return;
      }
      mp = p;
    }

    setSaving(true);
    try {
      await api.patch(`/api/admin/projects/${project.id}`, {
        weight: w,
        max_parallel: mp,
      });
      await onReload();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setRowError(err.message || err.code);
      } else if (err instanceof Error) {
        setRowError(err.message);
      } else {
        setRowError('Lỗi khi lưu cài đặt');
      }
    } finally {
      setSaving(false);
    }
  };

  const handleRecreateWebhook = async () => {
    setRecreatingWebhook(true);
    try {
      await api.post(`/api/admin/projects/${project.id}/webhook`);
      await onReload();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setRowError(err.message || err.code);
      } else if (err instanceof Error) {
        setRowError(err.message);
      } else {
        setRowError('Lỗi tạo webhook');
      }
    } finally {
      setRecreatingWebhook(false);
    }
  };

  const handleArchive = async () => {
    if (!window.confirm('Bạn có chắc muốn lưu trữ dự án này không?')) return;
    try {
      await api.patch(`/api/admin/projects/${project.id}`, { status: 'ARCHIVED' });
      await onReload();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setRowError(err.message || err.code);
      } else if (err instanceof Error) {
        setRowError(err.message);
      } else {
        setRowError('Lỗi khi lưu trữ dự án');
      }
    }
  };

  const handleRestore = async () => {
    try {
      await api.patch(`/api/admin/projects/${project.id}`, { status: 'ACTIVE' });
      await onReload();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setRowError(err.message || err.code);
      } else if (err instanceof Error) {
        setRowError(err.message);
      } else {
        setRowError('Lỗi khi khôi phục dự án');
      }
    }
  };

  const isArchived = project.status === 'ARCHIVED';

  return (
    <tr data-testid={`project-${project.id}`} className="border-b border-slate-200">
      <td className="px-4 py-3 font-medium text-slate-900">{project.github_repo}</td>
      <td className="px-4 py-3 text-slate-600">{project.default_branch}</td>
      <td className="px-4 py-3">
        {project.webhook_ok ? (
          <span className="font-semibold text-emerald-600">✓</span>
        ) : (
          <div className="flex items-center gap-2">
            <span className="font-semibold text-amber-500">⚠</span>
            <button
              type="button"
              disabled={recreatingWebhook}
              onClick={handleRecreateWebhook}
              className="rounded bg-slate-100 px-2 py-1 text-xs font-medium text-slate-700 hover:bg-slate-200 disabled:opacity-50"
            >
              Tạo lại webhook
            </button>
          </div>
        )}
      </td>
      <td className="px-4 py-3 text-slate-600">{project.open_tasks ?? 0}</td>
      <td className="px-4 py-3">
        <input
          type="number"
          aria-label="Trọng số"
          value={weight}
          onChange={(e) => {
            setWeight(e.target.value);
            if (rowError) setRowError(null);
          }}
          className="w-16 rounded border border-slate-300 px-2 py-1 text-xs"
        />
      </td>
      <td className="px-4 py-3">
        <input
          type="number"
          aria-label="Song song tối đa"
          value={maxParallel}
          onChange={(e) => {
            setMaxParallel(e.target.value);
            if (rowError) setRowError(null);
          }}
          placeholder="Trống"
          className="w-20 rounded border border-slate-300 px-2 py-1 text-xs"
        />
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            disabled={saving}
            onClick={handleSave}
            className="rounded bg-slate-900 px-2.5 py-1 text-xs font-medium text-white hover:bg-slate-800 disabled:opacity-50"
          >
            Lưu
          </button>
          {isArchived ? (
            <>
              <span className="rounded bg-slate-100 px-2 py-0.5 text-xs text-slate-600">
                Đã lưu trữ
              </span>
              <button
                type="button"
                onClick={handleRestore}
                className="rounded border border-slate-300 px-2 py-1 text-xs text-slate-700 hover:bg-slate-50"
              >
                Khôi phục
              </button>
            </>
          ) : (
            <button
              type="button"
              onClick={handleArchive}
              className="rounded border border-red-200 px-2 py-1 text-xs text-red-600 hover:bg-red-50"
            >
              Lưu trữ
            </button>
          )}
        </div>
        {rowError && <div className="mt-1 text-xs text-red-600">{rowError}</div>}
      </td>
    </tr>
  );
}

export default function Projects() {
  const [loading, setLoading] = useState(true);
  const [projects, setProjects] = useState<ProjectWithOps[]>([]);
  const [repos, setRepos] = useState<GitHubRepo[]>([]);
  const [projectsError, setProjectsError] = useState<string | null>(null);
  const [repoError, setRepoError] = useState<string | null>(null);
  const [addError, setAddError] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [addingRepo, setAddingRepo] = useState<string | null>(null);

  const loadData = useCallback(async () => {
    setProjectsError(null);
    setRepoError(null);

    try {
      const [pData, rData] = await Promise.all([
        api.get<ProjectWithOps[]>('/api/projects'),
        api.get<GitHubRepo[]>('/api/admin/github/repos').catch(() => {
          setRepoError(
            'Không đọc được repo từ GitHub — kiểm tra PAT: cần Metadata Read, Contents/Issues/Pull requests RW, Webhooks RW'
          );
          return [] as GitHubRepo[];
        }),
      ]);
      setProjects(pData || []);
      setRepos(rData || []);
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setProjectsError(err.message || err.code);
      } else if (err instanceof Error) {
        setProjectsError(err.message);
      } else {
        setProjectsError('Không thể tải danh sách dự án');
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const handleAddRepo = async (repoName: string) => {
    setAddingRepo(repoName);
    setAddError(null);
    setStatusMessage(null);
    try {
      const res = await api.post<{ webhook_ok?: boolean }>('/api/admin/projects', {
        github_repo: repoName,
      });
      if (res && res.webhook_ok === false) {
        setStatusMessage('Đã thêm, nhưng chưa tạo được webhook — bấm Tạo lại webhook');
      }
      await loadData();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setAddError(err.message || err.code);
      } else if (err instanceof Error) {
        setAddError(err.message);
      } else {
        setAddError('Không thể thêm dự án');
      }
    } finally {
      setAddingRepo(null);
    }
  };

  if (loading) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Dự án</h1>
        <Loading rows={4} />
      </div>
    );
  }

  if (projectsError) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Dự án</h1>
        <ErrorBanner
          message={projectsError}
          onRetry={() => {
            setLoading(true);
            loadData();
          }}
        />
      </div>
    );
  }

  const availableRepos = repos.filter((r) => !r.added);

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Dự án</h1>
        <p className="mt-1 text-sm text-slate-500">
          Quản lý các kho mã nguồn dự án, cấu hình trọng số và webhook tích hợp.
        </p>
      </div>

      {repoError && <ErrorBanner message={repoError} />}
      {addError && <ErrorBanner message={addError} />}
      {statusMessage && (
        <div
          role="status"
          className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800"
        >
          {statusMessage}
        </div>
      )}

      {projects.length === 0 ? (
        <div className="rounded-lg border border-dashed border-slate-300 py-8 text-center text-slate-600">
          Chưa có dự án
        </div>
      ) : (
        <div className="space-y-6">
          <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white shadow-xs">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-slate-200 bg-slate-50 text-slate-600">
                <tr>
                  <th className="px-4 py-3">Kho mã nguồn</th>
                  <th className="px-4 py-3">Nhánh mặc định</th>
                  <th className="px-4 py-3">Webhook</th>
                  <th className="px-4 py-3">Tác vụ mở</th>
                  <th className="px-4 py-3">Trọng số</th>
                  <th className="px-4 py-3">Song song tối đa</th>
                  <th className="px-4 py-3">Thao tác</th>
                </tr>
              </thead>
              <tbody>
                {projects.map((p) => (
                  <ProjectRow
                    key={`${p.id}-${p.weight}-${p.max_parallel}-${p.status}-${p.webhook_ok}`}
                    project={p}
                    onReload={loadData}
                  />
                ))}
              </tbody>
            </table>
          </div>

          <div className="space-y-6">
            {projects.map((p) => (
              <ProjectOps
                key={`ops-${p.id}`}
                project={p}
                onReload={loadData}
              />
            ))}
          </div>
        </div>
      )}

      <div data-testid="repos-available" className="space-y-4">
        <h2 className="text-lg font-semibold text-slate-900">Repo chưa thêm</h2>
        {availableRepos.length === 0 ? (
          <p className="text-xs text-slate-500">Không có repo nào khả dụng</p>
        ) : (
          <div className="divide-y divide-slate-200 rounded-xl border border-slate-200 bg-white shadow-xs">
            {availableRepos.map((r) => (
              <div
                key={r.full_name}
                className="flex items-center justify-between px-4 py-3 text-xs"
              >
                <div>
                  <span className="font-medium text-slate-900">{r.full_name}</span>
                  <span className="ml-2 text-slate-500">({r.default_branch})</span>
                </div>
                <button
                  type="button"
                  disabled={addingRepo === r.full_name}
                  onClick={() => handleAddRepo(r.full_name)}
                  className="rounded bg-slate-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-slate-800 disabled:opacity-50"
                >
                  {addingRepo === r.full_name ? 'Đang thêm...' : 'Thêm'}
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
