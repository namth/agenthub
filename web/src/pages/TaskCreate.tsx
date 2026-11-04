import { useState, useEffect, type FormEvent } from 'react';
import { useNavigate } from 'react-router';
import { api, ApiError } from '../api/client';
import { ErrorBanner } from '../components/StateView';
import { ALL_PROJECTS, getSelectedProject, type Project } from '../project';

export default function TaskCreate() {
  const navigate = useNavigate();
  const selectedProject = getSelectedProject();
  const [projects, setProjects] = useState<Project[]>([]);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [formProjectId, setFormProjectId] = useState('');
  const [titleError, setTitleError] = useState<string | null>(null);
  const [descError, setDescError] = useState<string | null>(null);
  const [projectError, setProjectError] = useState<string | null>(null);
  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (selectedProject !== null) {
      let alive = true;
      api
        .get<Project[]>('/api/projects')
        .then((data) => {
          if (alive) setProjects(data || []);
        })
        .catch(() => {});
      return () => {
        alive = false;
      };
    }
  }, [selectedProject]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setServerError(null);

    const trimmedTitle = title.trim();
    let hasError = false;

    if (selectedProject === ALL_PROJECTS) {
      if (!formProjectId) {
        setProjectError('Chọn dự án cho yêu cầu');
        hasError = true;
      } else {
        setProjectError(null);
      }
    }

    if (trimmedTitle.length < 6) {
      setTitleError('Tiêu đề tối thiểu 6 ký tự');
      hasError = true;
    } else if (trimmedTitle.length > 120) {
      setTitleError('Tiêu đề tối đa 120 ký tự');
      hasError = true;
    } else {
      setTitleError(null);
    }

    if (description.length < 20) {
      setDescError('Mô tả tối thiểu 20 ký tự');
      hasError = true;
    } else {
      setDescError(null);
    }

    if (hasError) {
      return;
    }

    setSubmitting(true);
    try {
      const payload: Record<string, unknown> = {
        title: trimmedTitle,
        description,
      };

      if (selectedProject && selectedProject !== ALL_PROJECTS) {
        payload.project_id = selectedProject;
      } else if (selectedProject === ALL_PROJECTS && formProjectId) {
        payload.project_id = formProjectId;
      }

      const res = await api.post<{ id: string }>('/api/tasks', payload);
      navigate(`/tasks/${res.id}`);
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setServerError(err.message || err.code);
      } else if (err instanceof Error) {
        setServerError(err.message);
      } else {
        setServerError('Lỗi máy chủ khi tạo tác vụ');
      }
    } finally {
      setSubmitting(false);
    }
  };

  const currentProject = projects.find((p) => p.id === selectedProject);

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Tạo yêu cầu mới</h1>
        <p className="mt-1 text-sm text-slate-500">
          Nhập thông tin yêu cầu để chuyển giao cho AI agent thực thi.
        </p>
      </div>

      {serverError && <ErrorBanner message={serverError} />}

      <form onSubmit={handleSubmit} className="space-y-6 rounded-xl border border-slate-200 bg-white p-6 shadow-xs">
        {selectedProject !== null && selectedProject !== ALL_PROJECTS && (
          <div>
            <span className="text-sm font-medium text-slate-700">
              Dự án: {currentProject?.github_repo ?? selectedProject}
            </span>
          </div>
        )}

        {selectedProject === ALL_PROJECTS && (
          <div>
            <label htmlFor="project-select" className="block text-sm font-medium text-slate-700">
              Dự án của yêu cầu
            </label>
            <div className="mt-1">
              <select
                id="project-select"
                aria-label="Dự án của yêu cầu"
                value={formProjectId}
                onChange={(e) => {
                  setFormProjectId(e.target.value);
                  if (projectError) setProjectError(null);
                }}
                className="block w-full rounded-md border border-slate-300 px-3 py-2 text-sm placeholder-slate-400 shadow-xs focus:border-slate-900 focus:outline-hidden focus:ring-1 focus:ring-slate-900"
              >
                <option value="">— Chọn dự án —</option>
                {projects
                  .filter((p) => p.status === 'ACTIVE')
                  .map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.github_repo}
                    </option>
                  ))}
              </select>
            </div>
            {projectError && <p className="mt-1 text-sm text-red-600">{projectError}</p>}
          </div>
        )}

        <div>
          <label htmlFor="title" className="block text-sm font-medium text-slate-700">
            Tiêu đề
          </label>
          <div className="mt-1">
            <input
              type="text"
              id="title"
              name="title"
              value={title}
              onChange={(e) => {
                setTitle(e.target.value);
                if (titleError) setTitleError(null);
              }}
              placeholder="Ví dụ: Xuất báo cáo PDF"
              className="block w-full rounded-md border border-slate-300 px-3 py-2 text-sm placeholder-slate-400 shadow-xs focus:border-slate-900 focus:outline-hidden focus:ring-1 focus:ring-slate-900"
            />
          </div>
          {titleError && <p className="mt-1 text-sm text-red-600">{titleError}</p>}
        </div>

        <div>
          <label htmlFor="description" className="block text-sm font-medium text-slate-700">
            Mô tả chi tiết
          </label>
          <div className="mt-1">
            <textarea
              id="description"
              name="description"
              rows={6}
              value={description}
              onChange={(e) => {
                setDescription(e.target.value);
                if (descError) setDescError(null);
              }}
              placeholder="Mô tả bối cảnh, yêu cầu kỹ thuật và tiêu chí nghiệm thu..."
              className="block w-full rounded-md border border-slate-300 px-3 py-2 text-sm placeholder-slate-400 shadow-xs focus:border-slate-900 focus:outline-hidden focus:ring-1 focus:ring-slate-900"
            />
          </div>
          {descError && <p className="mt-1 text-sm text-red-600">{descError}</p>}
        </div>

        <div className="flex justify-end gap-3">
          <button
            type="submit"
            disabled={submitting}
            className="inline-flex justify-center rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-xs hover:bg-slate-800 disabled:opacity-50"
          >
            {submitting ? 'Đang gửi...' : 'Gửi yêu cầu'}
          </button>
        </div>
      </form>
    </div>
  );
}
