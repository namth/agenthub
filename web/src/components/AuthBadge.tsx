export interface AuthBadgeProps {
  kind?: string;
  auth_status?: string | null;
  auth_detail?: string | null;
}

export default function AuthBadge({ kind, auth_status, auth_detail }: AuthBadgeProps) {
  if (auth_status === 'OK') {
    return null;
  }

  if (auth_status === 'NEEDS_AUTH') {
    const hint = kind === 'ANTIGRAVITY' ? 'Mở `agy` và đăng nhập' : 'Chạy `claude` rồi gõ /login';
    const title = auth_detail ? `${auth_detail} — ${hint}` : hint;

    return (
      <div className="space-y-1">
        <div>
          <span
            title={title}
            className="inline-flex items-center rounded bg-red-100 px-2 py-0.5 text-xs font-semibold text-red-800"
          >
            Cần đăng nhập
          </span>
        </div>
        <div className="text-xs text-red-600 font-mono">
          {hint}
        </div>
      </div>
    );
  }

  return (
    <div>
      <span className="inline-flex items-center rounded bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600">
        Chưa kiểm được
      </span>
    </div>
  );
}
