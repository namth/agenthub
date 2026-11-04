import { useState, useEffect, useCallback } from 'react';
import type { User } from '../hooks/useAuth';
import { api, ApiError } from '../api/client';
import { ErrorBanner, Loading } from '../components/StateView';

export interface TokenItem {
  id: string;
  name: string;
  last4: string;
  expires_at: string;
  last_used_at: string | null;
  created_at: string;
}

export interface CreatedToken extends TokenItem {
  token: string;
}

export interface ProfileProps {
  user: User;
}

export default function Profile({ user }: ProfileProps) {
  const isOwner = user.role === 'ADMIN';

  const [tokens, setTokens] = useState<TokenItem[]>([]);
  const [loading, setLoading] = useState<boolean>(isOwner);
  const [error, setError] = useState<string | null>(null);

  const [name, setName] = useState('');
  const [expiresDays, setExpiresDays] = useState(30);
  const [creating, setCreating] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [newToken, setNewToken] = useState<CreatedToken | null>(null);

  const loadTokens = useCallback(async () => {
    if (!isOwner) return;
    try {
      setError(null);
      const data = await api.get<TokenItem[]>('/api/me/tokens');
      setTokens(data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Lỗi khi tải danh sách token');
    } finally {
      setLoading(false);
    }
  }, [isOwner]);

  useEffect(() => {
    if (isOwner) {
      loadTokens();
    }
  }, [isOwner, loadTokens]);

  const handleCreateToken = async () => {
    if (!name.trim()) {
      return;
    }
    setCreating(true);
    setFormError(null);
    try {
      const created = await api.post<CreatedToken>('/api/me/tokens', {
        name: name.trim(),
        expires_days: Number(expiresDays),
      });
      setNewToken(created);
      setName('');
      await loadTokens();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'CONFLICT') {
        setFormError('Tên token đã tồn tại, vui lòng chọn tên khác.');
      } else {
        setFormError(err instanceof Error ? err.message : 'Lỗi khi tạo token');
      }
    } finally {
      setCreating(false);
    }
  };

  const handleRevoke = async (id: string) => {
    if (window.confirm('Bạn có chắc chắn muốn thu hồi token này?')) {
      try {
        await api.del(`/api/me/tokens/${id}`);
        if (newToken && newToken.id === id) {
          setNewToken(null);
        }
        await loadTokens();
      } catch (err) {
        alert('Lỗi khi thu hồi token: ' + (err instanceof Error ? err.message : String(err)));
      }
    }
  };

  return (
    <div className="max-w-4xl mx-auto space-y-8">
      {/* Thông tin tài khoản */}
      <div className="bg-white p-6 rounded-lg border border-slate-200 shadow-xs space-y-4">
        <h2 className="text-xl font-bold text-slate-900">Hồ sơ cá nhân</h2>
        <div className="flex items-center gap-4">
          {user.avatar_url ? (
            <img src={user.avatar_url} alt={user.username} className="w-16 h-16 rounded-full border border-slate-200" />
          ) : (
            <div className="w-16 h-16 rounded-full bg-slate-200 flex items-center justify-center font-bold text-slate-600 text-xl">
              {user.username.slice(0, 2).toUpperCase()}
            </div>
          )}
          <div>
            <div className="text-lg font-semibold text-slate-900">{user.username}</div>
            <div className="text-sm text-slate-500">
              Vai trò: <span className="font-medium text-slate-700">{user.role === 'ADMIN' ? 'Quản trị viên (Owner)' : 'Đối tác (Partner)'}</span>
            </div>
            <div className="text-xs text-slate-400">ID: {user.id}</div>
          </div>
        </div>
      </div>

      {/* Phần quản lý token cá nhân (chỉ hiển thị cho Owner/ADMIN) */}
      {isOwner && (
        <div className="bg-white p-6 rounded-lg border border-slate-200 shadow-xs space-y-6">
          <div>
            <h3 className="text-lg font-semibold text-slate-900">Token cá nhân & Cấu hình MCP</h3>
            <p className="text-sm text-slate-500 mt-1">
              Dùng token cá nhân để kết nối Claude Code hoặc các MCP client với AgentHub điều hành từ xa.
            </p>
          </div>

          {/* Form tạo token */}
          <div className="bg-slate-50 p-4 rounded-lg border border-slate-200 space-y-4">
            <h4 className="text-sm font-medium text-slate-900">Tạo token mới</h4>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
              <div className="sm:col-span-2 space-y-1">
                <label htmlFor="token-name" className="block text-xs font-medium text-slate-700">
                  Tên token
                </label>
                <input
                  id="token-name"
                  type="text"
                  placeholder="Ví dụ: macbook, claudecode..."
                  value={name}
                  onChange={(e) => {
                    setName(e.target.value);
                    if (formError) setFormError(null);
                  }}
                  className="w-full rounded-md border border-slate-300 px-3 py-1.5 text-sm focus:border-slate-500 focus:outline-hidden bg-white"
                />
              </div>
              <div className="space-y-1">
                <label htmlFor="token-expires" className="block text-xs font-medium text-slate-700">
                  Hết hạn
                </label>
                <select
                  id="token-expires"
                  value={expiresDays}
                  onChange={(e) => setExpiresDays(Number(e.target.value))}
                  className="w-full rounded-md border border-slate-300 px-3 py-1.5 text-sm focus:border-slate-500 focus:outline-hidden bg-white"
                >
                  <option value={30}>30 ngày</option>
                  <option value={90}>90 ngày</option>
                  <option value={365}>365 ngày</option>
                </select>
              </div>
            </div>

            {formError && (
              <div role="alert" className="p-3 text-sm text-red-700 bg-red-50 border border-red-200 rounded-md">
                {formError}
              </div>
            )}

            <div>
              <button
                type="button"
                onClick={handleCreateToken}
                disabled={creating}
                className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-50"
              >
                {creating ? 'Đang tạo…' : 'Tạo token'}
              </button>
            </div>
          </div>

          {/* Hộp hiển thị token mới tạo một lần duy nhất */}
          {newToken && (
            <div data-testid="new-token" className="p-4 bg-emerald-50 border border-emerald-200 rounded-lg space-y-3">
              <div className="flex items-center justify-between">
                <span className="font-semibold text-emerald-800">Token mới tạo</span>
                <button
                  type="button"
                  onClick={() => navigator.clipboard.writeText(newToken.token)}
                  className="text-xs px-2.5 py-1 bg-emerald-600 text-white rounded-md hover:bg-emerald-700 font-medium"
                >
                  Sao chép token
                </button>
              </div>
              <div className="font-mono text-sm break-all bg-white p-2.5 border border-emerald-200 rounded-md select-all text-emerald-950">
                {newToken.token}
              </div>
              <p className="text-xs text-amber-700 font-medium">
                Lưu ý: Token này chỉ hiển thị một lần duy nhất và sẽ không hiển thị lại! Hãy lưu vào nơi an toàn.
              </p>
              <div className="space-y-1.5 pt-2 border-t border-emerald-200">
                <p className="text-xs font-medium text-slate-700">Đoạn lệnh cấu hình Claude Code MCP:</p>
                <div className="font-mono text-xs bg-slate-900 text-slate-100 p-2.5 rounded-md overflow-x-auto whitespace-pre-wrap break-all">
                  {`claude mcp add --transport http agenthub ${window.location.origin}/mcp --header "Authorization: Bearer ${newToken.token}"`}
                </div>
                <button
                  type="button"
                  onClick={() =>
                    navigator.clipboard.writeText(
                      `claude mcp add --transport http agenthub ${window.location.origin}/mcp --header "Authorization: Bearer ${newToken.token}"`
                    )
                  }
                  className="text-xs px-2.5 py-1 bg-slate-700 text-white rounded-md hover:bg-slate-800 font-medium"
                >
                  Sao chép lệnh MCP
                </button>
              </div>
            </div>
          )}

          {/* Danh sách token */}
          <div className="space-y-3">
            <h4 className="text-sm font-medium text-slate-900">Danh sách token còn hiệu lực</h4>
            {loading ? (
              <Loading />
            ) : error ? (
              <ErrorBanner message={error} onRetry={loadTokens} />
            ) : tokens.length === 0 ? (
              <div className="p-8 text-center border border-dashed border-slate-300 rounded-lg text-slate-500 space-y-2">
                <p className="font-medium text-slate-700">Chưa có token</p>
                <p className="text-xs text-slate-500">
                  Dùng token cá nhân để kết nối Claude Code hoặc MCP client với AgentHub.
                </p>
              </div>
            ) : (
              <div className="divide-y divide-slate-200 border border-slate-200 rounded-lg overflow-hidden bg-white">
                {tokens.map((t) => (
                  <div
                    key={t.id}
                    data-testid={`token-${t.id}`}
                    className="p-4 flex items-center justify-between hover:bg-slate-50"
                  >
                    <div>
                      <div className="font-medium text-slate-900">{t.name}</div>
                      <div className="text-xs text-slate-500 space-x-2 mt-0.5">
                        <span className="font-mono bg-slate-100 px-1 py-0.5 rounded text-slate-700">…{t.last4}</span>
                        <span>·</span>
                        <span>Tạo: {new Date(t.created_at).toLocaleDateString()}</span>
                        <span>·</span>
                        <span>
                          {t.last_used_at ? `Dùng lần cuối: ${new Date(t.last_used_at).toLocaleDateString()}` : 'Chưa dùng'}
                        </span>
                        <span>·</span>
                        <span>Hết hạn: {new Date(t.expires_at).toLocaleDateString()}</span>
                      </div>
                    </div>
                    <button
                      type="button"
                      onClick={() => handleRevoke(t.id)}
                      className="text-xs px-3 py-1.5 border border-red-300 text-red-600 rounded-md hover:bg-red-50 font-medium"
                    >
                      Thu hồi
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
