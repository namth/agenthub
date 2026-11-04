import { useEffect, useState } from 'react';
import type { Role } from '../nav';

export interface User {
  id: string;
  username: string;
  avatar_url?: string;
  role: Role;
}

export type AuthState =
  | { status: 'loading' }
  | { status: 'anonymous' }
  | { status: 'error'; message: string }
  | { status: 'ready'; user: User };

// Gọi GET /api/auth/me (docs/07 mục 1.1). 401 → chưa đăng nhập.
export function useAuth(): AuthState {
  const [state, setState] = useState<AuthState>({ status: 'loading' });
  useEffect(() => {
    let alive = true;
    fetch('/api/auth/me', { credentials: 'include' })
      .then(async (res) => {
        if (!alive) return;
        if (res.status === 401 || res.status === 404) return setState({ status: 'anonymous' });
        const body = await res.json();
        if (!res.ok || !body.success) throw new Error(body.error?.message ?? `HTTP ${res.status}`);
        setState({ status: 'ready', user: body.data as User });
      })
      .catch((e: Error) => alive && setState({ status: 'error', message: e.message }));
    return () => {
      alive = false;
    };
  }, []);
  return state;
}
