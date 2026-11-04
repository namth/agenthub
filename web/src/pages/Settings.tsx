import { useCallback, useEffect, useState } from 'react';
import { api, ApiError } from '../api/client';
import { ErrorBanner, Loading } from '../components/StateView';

export interface SettingsProps {
  onOfficeChange?: (enabled: boolean) => void;
}

export default function Settings({ onOfficeChange }: SettingsProps = {}) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [officeEnabled, setOfficeEnabled] = useState(false);
  const [telegramEnabled, setTelegramEnabled] = useState(false);
  const [saving, setSaving] = useState(false);

  const fetchSettings = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.get<{ office_enabled: boolean; telegram_enabled?: boolean }>('/api/admin/settings');
      setOfficeEnabled(Boolean(data?.office_enabled));
      setTelegramEnabled(Boolean(data?.telegram_enabled));
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(err.message || err.code);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Không thể tải cài đặt');
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSettings();
  }, [fetchSettings]);

  const handleToggle = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const nextVal = e.target.checked;
    setError(null);
    setSaving(true);
    try {
      const res = await api.patch<{ office_enabled: boolean }>('/api/admin/settings', { office_enabled: nextVal });
      setOfficeEnabled(res.office_enabled);
      onOfficeChange?.(res.office_enabled);
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(err.message || err.code);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Cập nhật cài đặt thất bại');
      }
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="space-y-4">
        <Loading rows={3} />
      </div>
    );
  }

  return (
    <section className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Cài đặt hệ thống</h1>
        <p className="mt-1 text-sm text-slate-500">
          Quản lý các tính năng và tham số hệ thống dành cho Quản trị viên.
        </p>
      </div>

      {error && <ErrorBanner message={error} onRetry={fetchSettings} />}

      <div className="rounded-xl border border-slate-200 bg-white p-6 shadow-xs">
        <h2 className="mb-4 text-base font-semibold text-slate-900">Tính năng mở rộng</h2>
        <div className="flex items-start gap-3">
          <label className="flex items-center gap-3 cursor-pointer select-none">
            <input
              type="checkbox"
              checked={officeEnabled}
              onChange={handleToggle}
              disabled={saving}
              className="h-4 w-4 rounded border-slate-300 text-slate-900 focus:ring-slate-900"
            />
            <span className="text-sm font-medium text-slate-800">Bật văn phòng ảo</span>
          </label>
        </div>
        <p className="mt-1 pl-7 text-xs text-slate-500">
          Hiển thị màn hình quan sát văn phòng ảo (SCR-08) mô phỏng dàn máy và agent đang làm việc trong thời gian thực.
        </p>
        <div className="mt-4 pt-4 border-t border-slate-100">
          <p className="text-sm text-slate-700">
            {telegramEnabled
              ? 'Telegram: đang bật (cấu hình trong .env)'
              : 'Telegram: đang tắt (cấu hình trong .env)'}
          </p>
        </div>
      </div>
    </section>
  );
}
