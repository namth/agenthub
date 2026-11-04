import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router';
import { api, ApiError } from '../api/client';
import { ErrorBanner } from '../components/StateView';

export default function TaskCreate() {
  const navigate = useNavigate();
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [titleError, setTitleError] = useState<string | null>(null);
  const [descError, setDescError] = useState<string | null>(null);
  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setServerError(null);

    const trimmedTitle = title.trim();
    let hasError = false;

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
      const res = await api.post<{ id: string }>('/api/tasks', {
        title: trimmedTitle,
        description,
      });
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

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Tạo yêu cầu mới</h1>
        <p className="mt-1 text-sm text-slate-500">
          Nhập thông tin yêu cầu để chuyển giao cho AI agent thực thi.
        </p>
      </div>

      {serverError && <ErrorBanner message={serverError} />}

      <form onSubmit={handleSubmit} className="space-y-6 rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
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
              className="block w-full rounded-md border border-slate-300 px-3 py-2 text-sm placeholder-slate-400 shadow-sm focus:border-slate-900 focus:outline-none focus:ring-1 focus:ring-slate-900"
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
              className="block w-full rounded-md border border-slate-300 px-3 py-2 text-sm placeholder-slate-400 shadow-sm focus:border-slate-900 focus:outline-none focus:ring-1 focus:ring-slate-900"
            />
          </div>
          {descError && <p className="mt-1 text-sm text-red-600">{descError}</p>}
        </div>

        <div className="flex justify-end gap-3">
          <button
            type="submit"
            disabled={submitting}
            className="inline-flex justify-center rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800 disabled:opacity-50"
          >
            {submitting ? 'Đang gửi...' : 'Gửi yêu cầu'}
          </button>
        </div>
      </form>
    </div>
  );
}
