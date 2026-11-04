import { useCallback, useEffect, useState } from 'react';
import { api, ApiError } from '../api/client';
import { Empty, ErrorBanner, Loading } from '../components/StateView';

export interface ContributionRow {
  key: string;
  label: string;
  machines: number;
  jobs: {
    done: number;
    failed: number;
    exhausted: number;
    by_kind: Record<string, number>;
  };
  tasks_done: number;
  prs_merged: number;
  busy_hours: number;
}

export interface TimelineItem {
  at: string;
  worker: string;
  agent: string;
  job_kind: string;
  task_id: string;
  task_title: string;
  project: string;
  result: string;
  attempt: number;
}

export default function Contributions() {
  const [by, setBy] = useState<'partner' | 'project'>('partner');
  const [days, setDays] = useState<number>(7);
  const [rows, setRows] = useState<ContributionRow[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  const [selectedRow, setSelectedRow] = useState<ContributionRow | null>(null);
  const [timeline, setTimeline] = useState<TimelineItem[]>([]);
  const [timelineLoading, setTimelineLoading] = useState<boolean>(false);
  const [timelineError, setTimelineError] = useState<string | null>(null);

  const getDateRange = useCallback((rangeDays: number) => {
    const toDate = new Date();
    const fromDate = new Date(toDate.getTime() - rangeDays * 86400000);
    return {
      from: fromDate.toISOString(),
      to: toDate.toISOString(),
    };
  }, []);

  const fetchContributions = useCallback(async () => {
    setLoading(true);
    setError(null);
    setSelectedRow(null);
    setTimeline([]);
    try {
      const { from, to } = getDateRange(days);
      const url = `/api/contributions?by=${by}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;
      const data = await api.get<ContributionRow[]>(url);
      setRows(data ?? []);
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(err.message || err.code);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Không tải được dữ liệu đóng góp');
      }
    } finally {
      setLoading(false);
    }
  }, [by, days, getDateRange]);

  useEffect(() => {
    fetchContributions();
  }, [fetchContributions]);

  const fetchTimeline = useCallback(
    async (row: ContributionRow) => {
      setTimelineLoading(true);
      setTimelineError(null);
      try {
        const { from, to } = getDateRange(days);
        const paramKey = by === 'partner' ? 'partner' : 'project';
        const url = `/api/contributions/timeline?${paramKey}=${encodeURIComponent(row.key)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;
        const data = await api.get<TimelineItem[]>(url);
        setTimeline(data ?? []);
      } catch (err: unknown) {
        if (err instanceof ApiError) {
          setTimelineError(err.message || err.code);
        } else if (err instanceof Error) {
          setTimelineError(err.message);
        } else {
          setTimelineError('Không tải được dòng thời gian');
        }
      } finally {
        setTimelineLoading(false);
      }
    },
    [by, days, getDateRange]
  );

  const handleSelectRow = (row: ContributionRow) => {
    setSelectedRow(row);
    fetchTimeline(row);
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Đóng góp</h1>

        <div className="flex flex-wrap items-center gap-6">
          <div className="flex items-center gap-4">
            <label className="flex cursor-pointer items-center gap-2 text-sm font-medium text-slate-700">
              <input
                type="radio"
                role="radio"
                name="by"
                value="partner"
                checked={by === 'partner'}
                onChange={() => setBy('partner')}
                className="h-4 w-4 text-indigo-600 focus:ring-indigo-500"
              />
              <span>Partner</span>
            </label>
            <label className="flex cursor-pointer items-center gap-2 text-sm font-medium text-slate-700">
              <input
                type="radio"
                role="radio"
                name="by"
                value="project"
                checked={by === 'project'}
                onChange={() => setBy('project')}
                className="h-4 w-4 text-indigo-600 focus:ring-indigo-500"
              />
              <span>Dự án</span>
            </label>
          </div>

          <div className="flex items-center gap-2">
            <label htmlFor="range-select" className="text-sm font-medium text-slate-700">
              Khoảng thời gian
            </label>
            <select
              id="range-select"
              value={days}
              onChange={(e) => setDays(Number(e.target.value))}
              className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm text-slate-900 shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
            >
              <option value="7">7 ngày</option>
              <option value="30">30 ngày</option>
              <option value="90">90 ngày</option>
            </select>
          </div>
        </div>
      </div>

      {loading ? (
        <Loading rows={4} />
      ) : error ? (
        <ErrorBanner message={error} onRetry={fetchContributions} />
      ) : rows.length === 0 ? (
        <Empty title="Chưa có đóng góp trong khoảng này" />
      ) : (
        <div className="overflow-x-auto rounded-lg border border-slate-200 shadow-sm">
          <table className="min-w-full divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50 text-slate-600 font-medium">
              <tr>
                <th scope="col" className="px-4 py-3 text-left">Tên</th>
                <th scope="col" className="px-4 py-3 text-right">Máy</th>
                <th scope="col" className="px-4 py-3 text-right">Job xong</th>
                <th scope="col" className="px-4 py-3 text-right">Lỗi/limit</th>
                <th scope="col" className="px-4 py-3 text-right">Task DONE</th>
                <th scope="col" className="px-4 py-3 text-right">PR merge</th>
                <th scope="col" className="px-4 py-3 text-right">Giờ máy</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 bg-white">
              {rows.map((r) => {
                const isSelected = selectedRow?.key === r.key;
                return (
                  <tr
                    key={r.key}
                    onClick={() => handleSelectRow(r)}
                    className={`cursor-pointer transition-colors ${
                      isSelected ? 'bg-indigo-50 font-medium' : 'hover:bg-slate-50'
                    }`}
                  >
                    <td className="whitespace-nowrap px-4 py-3 text-slate-900 font-medium">{r.label}</td>
                    <td className="whitespace-nowrap px-4 py-3 text-right text-slate-600">{r.machines}</td>
                    <td className="whitespace-nowrap px-4 py-3 text-right text-slate-600">{r.jobs.done}</td>
                    <td className="whitespace-nowrap px-4 py-3 text-right text-slate-600">
                      {r.jobs.failed} / {r.jobs.exhausted}
                    </td>
                    <td className="whitespace-nowrap px-4 py-3 text-right text-slate-600">{r.tasks_done}</td>
                    <td className="whitespace-nowrap px-4 py-3 text-right text-slate-600">{r.prs_merged}</td>
                    <td className="whitespace-nowrap px-4 py-3 text-right text-slate-600">{r.busy_hours}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {selectedRow && (
        <section className="mt-8 space-y-4 rounded-xl border border-slate-200 bg-slate-50/50 p-5">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold text-slate-900">
              Dòng thời gian: {selectedRow.label}
            </h2>
            <button
              type="button"
              onClick={() => setSelectedRow(null)}
              className="rounded-md border border-slate-300 bg-white px-2.5 py-1 text-xs font-medium text-slate-700 shadow-sm hover:bg-slate-50"
            >
              Đóng
            </button>
          </div>

          {timelineLoading ? (
            <Loading rows={2} />
          ) : timelineError ? (
            <ErrorBanner message={timelineError} onRetry={() => fetchTimeline(selectedRow)} />
          ) : timeline.length === 0 ? (
            <p className="text-sm text-slate-500">Chưa có hoạt động nào trong khoảng thời gian này.</p>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white">
              <table className="min-w-full divide-y divide-slate-200 text-sm">
                <thead className="bg-slate-50 text-slate-600 font-medium">
                  <tr>
                    <th scope="col" className="px-4 py-2.5 text-left">Thời điểm</th>
                    <th scope="col" className="px-4 py-2.5 text-left">Máy / Agent</th>
                    <th scope="col" className="px-4 py-2.5 text-left">Loại job</th>
                    <th scope="col" className="px-4 py-2.5 text-left">Tên task</th>
                    <th scope="col" className="px-4 py-2.5 text-left">Dự án</th>
                    <th scope="col" className="px-4 py-2.5 text-left">Kết quả</th>
                    <th scope="col" className="px-4 py-2.5 text-right">Lần thử</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {timeline.map((item, idx) => (
                    <tr key={idx} className="hover:bg-slate-50">
                      <td className="whitespace-nowrap px-4 py-2.5 text-slate-600">
                        {new Date(item.at).toLocaleString()}
                      </td>
                      <td className="whitespace-nowrap px-4 py-2.5 font-mono text-xs text-slate-700">
                        {item.worker} ({item.agent})
                      </td>
                      <td className="whitespace-nowrap px-4 py-2.5 text-slate-800 font-medium">
                        {item.job_kind}
                      </td>
                      <td className="px-4 py-2.5 font-medium text-slate-900">
                        {item.task_title}
                      </td>
                      <td className="whitespace-nowrap px-4 py-2.5 text-slate-600">
                        {item.project}
                      </td>
                      <td className="whitespace-nowrap px-4 py-2.5">
                        <span
                          className={`inline-block rounded px-2 py-0.5 text-xs font-semibold ${
                            item.result === 'SUBMITTED'
                              ? 'bg-emerald-100 text-emerald-800'
                              : item.result === 'EXHAUSTED'
                              ? 'bg-amber-100 text-amber-800'
                              : 'bg-red-100 text-red-800'
                          }`}
                        >
                          {item.result}
                        </span>
                      </td>
                      <td className="whitespace-nowrap px-4 py-2.5 text-right text-slate-600">
                        {item.attempt}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      )}
    </div>
  );
}
