import { useState } from 'react';
import { api, ApiError } from '../api/client';

export interface ClarifyQuestion {
  id: string;
  question: string;
  options: string[];
  recommended: string;
  reason?: string;
}

export interface ClarifyRound {
  round: number;
  status: 'WAITING' | 'ANSWERED' | 'AUTO_DECIDED';
  questions: ClarifyQuestion[];
  answers?: Record<string, string> | null;
  answered_by?: string | null;
  answered_via?: string | null;
  answered_at?: string | null;
}

export interface ClarifyData {
  rounds: ClarifyRound[];
  can_answer: boolean;
  can_auto: boolean;
}

export interface ClarifyPanelProps {
  taskId: string;
  taskStatus: string;
  data: ClarifyData;
  onReload: () => Promise<void>;
}

export default function ClarifyPanel({ taskId, taskStatus, data, onReload }: ClarifyPanelProps) {
  const [selectedOption, setSelectedOption] = useState<Record<string, string>>({});
  const [customText, setCustomText] = useState<Record<string, string>>({});
  const [notes, setNotes] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [autoDeciding, setAutoDeciding] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  const waitingRound = data.rounds.find((r) => r.status === 'WAITING');
  const historyRounds = data.rounds.filter(
    (r) => r.status === 'ANSWERED' || r.status === 'AUTO_DECIDED'
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!waitingRound || !data.can_answer || submitting) return;

    setErrorMessage(null);
    setSuccessMessage(null);

    const answersPayload: Record<string, string> = {};
    for (const q of waitingRound.questions) {
      const hasOptions = Array.isArray(q.options) && q.options.length > 0;
      if (hasOptions) {
        if (selectedOption[q.id] === '__OTHER__') {
          const val = customText[q.id]?.trim() || '';
          if (!val) {
            setErrorMessage('Hãy trả lời đủ các câu hỏi');
            return;
          }
          answersPayload[q.id] = val;
        } else if (selectedOption[q.id]) {
          answersPayload[q.id] = selectedOption[q.id];
        } else {
          setErrorMessage('Hãy trả lời đủ các câu hỏi');
          return;
        }
      } else {
        const val = customText[q.id]?.trim() || '';
        if (!val) {
          setErrorMessage('Hãy trả lời đủ các câu hỏi');
          return;
        }
        answersPayload[q.id] = val;
      }
    }

    if (notes.trim()) {
      answersPayload._notes = notes.trim();
    }

    setSubmitting(true);
    try {
      await api.post(`/api/tasks/${taskId}/clarify`, { answers: answersPayload });
      setSuccessMessage('Đã gửi — trưởng phòng đang hoàn thiện kế hoạch');
      await onReload();
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 409) {
        setErrorMessage(err.message || 'Câu hỏi đã được trả lời');
        await onReload();
      } else if (err instanceof ApiError) {
        setErrorMessage(err.message || 'Không thể gửi câu trả lời');
      } else if (err instanceof Error) {
        setErrorMessage(err.message);
      } else {
        setErrorMessage('Không thể gửi câu trả lời');
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleAutoDecide = async () => {
    if (!data.can_auto || !waitingRound || autoDeciding) return;
    if (!window.confirm('Cho trưởng phòng tự quyết định phương án?')) return;

    setAutoDeciding(true);
    setErrorMessage(null);
    try {
      await api.post(`/api/tasks/${taskId}/clarify/auto`);
      await onReload();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setErrorMessage(err.message || 'Lỗi xử lý tự quyết');
      } else if (err instanceof Error) {
        setErrorMessage(err.message);
      } else {
        setErrorMessage('Lỗi xử lý tự quyết');
      }
    } finally {
      setAutoDeciding(false);
    }
  };

  return (
    <section data-testid="clarify" className="space-y-6 rounded-xl border border-slate-200 bg-white p-6 shadow-xs">
      {taskStatus === 'WAITING_USER' && (
        <div
          role="status"
          className="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm font-medium text-amber-800"
        >
          Trưởng phòng cần bạn trả lời câu hỏi làm rõ trước khi chia việc.
        </div>
      )}

      {successMessage && (
        <div
          role="status"
          className="rounded-lg border border-emerald-200 bg-emerald-50 p-4 text-sm font-medium text-emerald-800"
        >
          {successMessage}
        </div>
      )}

      {errorMessage && (
        <div
          role="alert"
          className="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm font-medium text-rose-800"
        >
          {errorMessage}
        </div>
      )}

      {waitingRound && (
        <form onSubmit={handleSubmit} className="space-y-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between border-b border-slate-100 pb-3">
            <h2 className="text-base font-semibold text-slate-800">
              Câu hỏi làm rõ (Vòng {waitingRound.round})
            </h2>
            {data.can_auto && (
              <button
                type="button"
                onClick={handleAutoDecide}
                disabled={autoDeciding}
                className="min-h-11 rounded-lg border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-700 shadow-xs hover:bg-slate-50 disabled:opacity-50"
              >
                {autoDeciding ? 'Đang gửi…' : 'Cho trưởng phòng tự quyết'}
              </button>
            )}
          </div>

          <div className="space-y-5">
            {waitingRound.questions.map((q) => {
              const hasOptions = Array.isArray(q.options) && q.options.length > 0;
              return (
                <fieldset
                  key={q.id}
                  className="rounded-lg border border-slate-200 bg-slate-50/50 p-4"
                >
                  <legend className="px-1 text-sm font-semibold text-slate-800">
                    {q.question}
                  </legend>
                  {q.reason && (
                    <p className="mt-1 text-xs text-slate-500">{q.reason}</p>
                  )}

                  <div className="mt-3 space-y-2">
                    {hasOptions && (
                      <>
                        {q.options.map((opt) => {
                          const isRecommended = opt === q.recommended;
                          const labelText = isRecommended ? `${opt} (Đề xuất)` : opt;
                          return (
                            <label
                              key={opt}
                              className="flex cursor-pointer items-center gap-2 text-sm text-slate-700"
                            >
                              <input
                                type="radio"
                                name={`clarify_${q.id}`}
                                value={opt}
                                checked={selectedOption[q.id] === opt}
                                disabled={!data.can_answer}
                                onChange={() => {
                                  setSelectedOption((prev) => ({ ...prev, [q.id]: opt }));
                                }}
                                className="h-4 w-4 text-blue-600 focus:ring-blue-500"
                              />
                              <span>{labelText}</span>
                            </label>
                          );
                        })}

                        <label className="flex cursor-pointer items-center gap-2 text-sm text-slate-700">
                          <input
                            type="radio"
                            name={`clarify_${q.id}`}
                            value="__OTHER__"
                            checked={selectedOption[q.id] === '__OTHER__'}
                            disabled={!data.can_answer}
                            onChange={() => {
                              setSelectedOption((prev) => ({ ...prev, [q.id]: '__OTHER__' }));
                            }}
                            className="h-4 w-4 text-blue-600 focus:ring-blue-500"
                          />
                          <span>Khác</span>
                        </label>
                      </>
                    )}

                    {(!hasOptions || selectedOption[q.id] === '__OTHER__') && (
                      <div className={hasOptions ? 'mt-2 pl-6' : 'mt-1'}>
                        <input
                          type="text"
                          aria-label={`Khác: ${q.question}`}
                          value={customText[q.id] || ''}
                          disabled={!data.can_answer}
                          onChange={(e) => {
                            setCustomText((prev) => ({ ...prev, [q.id]: e.target.value }));
                          }}
                          placeholder="Nhập câu trả lời..."
                          className="min-h-11 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 shadow-xs focus:border-blue-500 focus:outline-hidden focus:ring-1 focus:ring-blue-500 disabled:bg-slate-100 disabled:text-slate-500"
                        />
                      </div>
                    )}
                  </div>
                </fieldset>
              );
            })}
          </div>

          <div className="space-y-1">
            <label htmlFor="clarify-notes" className="text-sm font-medium text-slate-700">
              Ghi chú thêm
            </label>
            <textarea
              id="clarify-notes"
              aria-label="Ghi chú thêm"
              value={notes}
              disabled={!data.can_answer}
              onChange={(e) => setNotes(e.target.value)}
              maxLength={1000}
              rows={3}
              placeholder="Ghi chú thêm cho câu trả lời (tuỳ chọn, tối đa 1000 ký tự)..."
              className="w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 shadow-xs focus:border-blue-500 focus:outline-hidden focus:ring-1 focus:ring-blue-500 disabled:bg-slate-100 disabled:text-slate-500"
            />
          </div>

          {data.can_answer && (
            <div className="flex justify-end gap-3 pt-2">
              <button
                type="submit"
                disabled={submitting}
                className="min-h-11 rounded-lg bg-blue-600 px-5 py-2.5 text-sm font-medium text-white shadow-xs hover:bg-blue-700 disabled:opacity-50"
              >
                {submitting ? 'Đang gửi…' : 'Gửi câu trả lời'}
              </button>
            </div>
          )}
        </form>
      )}

      {historyRounds.length > 0 && (
        <div data-testid="clarify-history" className="space-y-4 border-t border-slate-200 pt-6">
          <h3 className="text-sm font-semibold tracking-wider text-slate-500 uppercase">
            Lịch sử làm rõ
          </h3>
          <div className="space-y-4">
            {historyRounds.map((hr) => (
              <div key={hr.round} className="rounded-lg border border-slate-200 bg-slate-50/70 p-4">
                <div className="mb-2 flex flex-wrap items-center justify-between gap-2 text-xs text-slate-500">
                  <span className="font-semibold text-slate-700">Vòng {hr.round}</span>
                  <div className="flex items-center gap-2">
                    {hr.answered_by && <span>Người trả lời: {hr.answered_by}</span>}
                    {hr.answered_via && (
                      <span className="rounded bg-slate-200 px-1.5 py-0.5 font-mono text-[10px] text-slate-700">
                        {hr.answered_via}
                      </span>
                    )}
                    {hr.answered_at && <span>{hr.answered_at}</span>}
                  </div>
                </div>

                <div className="space-y-2">
                  {hr.questions.map((q) => (
                    <div key={q.id} className="text-sm">
                      <span className="font-medium text-slate-800">{q.question}</span>
                      <span className="mx-2 text-slate-400">→</span>
                      <span className="text-slate-900">{hr.answers?.[q.id] || '(Không có câu trả lời)'}</span>
                    </div>
                  ))}
                  {hr.answers?._notes && (
                    <div className="mt-2 rounded bg-white p-2 text-xs text-slate-600">
                      <span className="font-medium text-slate-500">Ghi chú:</span> {hr.answers._notes}
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </section>
  );
}
