import { useState, useEffect, useCallback, useRef } from 'react';
import { api, ApiError } from '../api/client';
import { ErrorBanner } from './StateView';

interface AddMachineDialogProps {
  open: boolean;
  onClose: () => void;
}

interface JoinCodeData {
  code: string;
  expires_at: string;
  command: string;
  agent_prompt: string;
}

interface MachineSummary {
  id: string;
  name: string;
  conn_status: string;
}

export default function AddMachineDialog({ open, onClose }: AddMachineDialogProps) {
  const [name, setName] = useState('');
  const [nameError, setNameError] = useState<string | null>(null);
  const [apiError, setApiError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [joinData, setJoinData] = useState<JoinCodeData | null>(null);
  const [isExpired, setIsExpired] = useState(false);
  const [remainingSecs, setRemainingSecs] = useState<number>(0);
  const [connected, setConnected] = useState(false);
  const [copiedType, setCopiedType] = useState<'cmd' | 'prompt' | null>(null);

  // Reset state when dialog opens/closes
  useEffect(() => {
    if (!open) {
      setName('');
      setNameError(null);
      setApiError(null);
      setLoading(false);
      setJoinData(null);
      setIsExpired(false);
      setRemainingSecs(0);
      setConnected(false);
      setCopiedType(null);
    }
  }, [open]);

  // Request join code
  const fetchJoinCode = useCallback(async (machineName: string) => {
    setLoading(true);
    setApiError(null);
    try {
      const data = await api.post<JoinCodeData>('/api/fleet/join-codes', { name: machineName });
      setJoinData(data);
      setConnected(false);

      const diff = Math.floor((new Date(data.expires_at).getTime() - Date.now()) / 1000);
      if (diff <= 0) {
        setIsExpired(true);
        setRemainingSecs(0);
      } else {
        setIsExpired(false);
        setRemainingSecs(diff);
      }
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setApiError(err.message || err.code);
      } else if (err instanceof Error) {
        setApiError(err.message);
      } else {
        setApiError('Lỗi khi tạo mã tham gia');
      }
    } finally {
      setLoading(false);
    }
  }, []);

  const handleCreate = async () => {
    if (!/^[a-zA-Z0-9-]{2,40}$/.test(name)) {
      setNameError('Tên máy 2–40 ký tự: chữ không dấu, số, gạch ngang');
      return;
    }
    setNameError(null);
    await fetchJoinCode(name);
  };

  const handleCreateNew = async () => {
    if (!name) return;
    await fetchJoinCode(name);
  };

  // Timer countdown
  useEffect(() => {
    if (!joinData || isExpired) return;

    const timer = setInterval(() => {
      const diff = Math.floor((new Date(joinData.expires_at).getTime() - Date.now()) / 1000);
      if (diff <= 0) {
        setIsExpired(true);
        setRemainingSecs(0);
        clearInterval(timer);
      } else {
        setRemainingSecs(diff);
      }
    }, 1000);

    return () => clearInterval(timer);
  }, [joinData, isExpired]);

  // Polling for machine connection
  const checkRef = useRef<() => Promise<void>>(async () => {});
  checkRef.current = async () => {
    try {
      const machines = await api.get<MachineSummary[]>('/api/my/machines');
      const found = machines?.some((m) => m.name === name && m.conn_status === 'ONLINE');
      if (found) {
        setConnected(true);
      }
    } catch {
      // ignore polling errors
    }
  };

  useEffect(() => {
    if (!joinData || isExpired || connected) {
      return;
    }

    // Call immediately
    checkRef.current();

    const interval = setInterval(() => {
      checkRef.current();
    }, 3000);

    return () => clearInterval(interval);
  }, [joinData, isExpired, connected]);

  const handleCopy = async (text: string, type: 'cmd' | 'prompt') => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedType(type);
      setTimeout(() => setCopiedType(null), 2000);
    } catch {
      // ignore clipboard error
    }
  };

  if (!open) return null;

  const mm = String(Math.floor(remainingSecs / 60)).padStart(2, '0');
  const ss = String(remainingSecs % 60).padStart(2, '0');

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="add-machine-dialog-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
    >
      <div className="w-full max-w-2xl rounded-2xl bg-white p-6 shadow-xl space-y-5 max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between border-b border-slate-200 pb-3">
          <h2 id="add-machine-dialog-title" className="text-lg font-bold text-slate-900">
            Thêm máy mới
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50"
          >
            Đóng
          </button>
        </div>

        {apiError && <ErrorBanner message={apiError} />}

        {!joinData ? (
          <div className="space-y-4">
            <div>
              <label htmlFor="add-machine-input" className="block text-sm font-medium text-slate-700 mb-1">
                Tên máy
              </label>
              <input
                id="add-machine-input"
                type="text"
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  setNameError(null);
                }}
                placeholder="ví dụ: mac-moi, vps-ha-noi"
                className="w-full rounded-lg border border-slate-300 px-3.5 py-2 text-sm shadow-xs focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
              />
              {nameError && (
                <p className="mt-1 text-xs text-red-600">{nameError}</p>
              )}
            </div>

            <div className="flex justify-end">
              <button
                type="button"
                onClick={handleCreate}
                disabled={loading}
                className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white shadow-xs hover:bg-indigo-700 disabled:opacity-50"
              >
                {loading ? 'Đang tạo...' : 'Tạo mã'}
              </button>
            </div>
          </div>
        ) : (
          <div className="space-y-5">
            {/* Countdown / Connection status bar */}
            <div className="flex items-center justify-between rounded-lg bg-slate-50 p-3 text-sm">
              <div>
                {connected ? (
                  <span className="font-semibold text-emerald-600">Đã kết nối ✓</span>
                ) : isExpired ? (
                  <span className="font-semibold text-red-600">Mã đã hết hạn</span>
                ) : (
                  <span className="text-slate-600">Đang chờ máy kết nối…</span>
                )}
              </div>
              <div>
                {isExpired ? (
                  <button
                    type="button"
                    onClick={handleCreateNew}
                    disabled={loading}
                    className="rounded-md bg-indigo-600 px-3 py-1 text-xs font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
                  >
                    Tạo mã mới
                  </button>
                ) : (
                  <span className="font-mono text-slate-500">
                    Còn {mm}:{ss}
                  </span>
                )}
              </div>
            </div>

            {/* Step 2 (1): One-line install command */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold uppercase tracking-wider text-slate-500">
                  Lệnh cài một dòng
                </span>
                <button
                  type="button"
                  onClick={() => handleCopy(joinData.command, 'cmd')}
                  className="rounded border border-slate-200 bg-white px-2.5 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50"
                >
                  {copiedType === 'cmd' ? 'Đã sao chép ✓' : 'Sao chép lệnh'}
                </button>
              </div>
              <pre
                data-testid="join-command"
                className="overflow-x-auto rounded-lg bg-slate-900 p-3 font-mono text-xs text-slate-100 whitespace-pre-wrap break-all"
              >
                {joinData.command}
              </pre>
            </div>

            {/* Step 2 (2): Agent prompt */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold uppercase tracking-wider text-slate-500">
                  Hướng dẫn cho agent (Claude Code / Antigravity)
                </span>
                <button
                  type="button"
                  onClick={() => handleCopy(joinData.agent_prompt, 'prompt')}
                  className="rounded border border-slate-200 bg-white px-2.5 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50"
                >
                  {copiedType === 'prompt' ? 'Đã sao chép ✓' : 'Sao chép hướng dẫn'}
                </button>
              </div>
              <pre
                data-testid="join-prompt"
                className="max-h-48 overflow-y-auto rounded-lg bg-slate-900 p-3 font-mono text-xs text-slate-100 whitespace-pre-wrap break-all"
              >
                {joinData.agent_prompt}
              </pre>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
