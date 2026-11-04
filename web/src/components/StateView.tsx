import type { ReactNode } from 'react';

// Giao thức 4 trạng thái UI (docs/03 mục 3).
export function Loading({ rows = 3 }: { rows?: number }) {
  return (
    <div className="space-y-3" aria-busy="true">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="h-12 animate-pulse rounded-lg bg-slate-200" />
      ))}
    </div>
  );
}

export function Empty({ title, action }: { title: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-4 rounded-xl border border-dashed border-slate-300 py-16 text-center">
      <p className="text-slate-600">{title}</p>
      {action}
    </div>
  );
}

export function ErrorBanner({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div role="alert" className="flex items-center justify-between rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-red-800">
      <span>{message}</span>
      {onRetry && (
        <button onClick={onRetry} className="rounded-md bg-red-600 px-3 py-1 text-sm text-white hover:bg-red-700">
          Thử lại
        </button>
      )}
    </div>
  );
}
