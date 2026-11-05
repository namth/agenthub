export class ApiError extends Error {
  code: string;
  status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
  }
}

export interface ApiResponse<T> {
  success: boolean;
  data: T | null;
  error: { code: string; message: string } | null;
}

export async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const options: RequestInit = {
    credentials: 'include',
    ...init,
  };

  const res = await fetch(url, options);
  if (res.status === 204) {
    return undefined as unknown as T;
  }
  let body: ApiResponse<T> | null = null;
  try {
    body = (await res.json()) as ApiResponse<T>;
  } catch {
    throw new ApiError(`HTTP ${res.status}`, 'HTTP_ERROR', res.status);
  }

  if (!res.ok || !body?.success) {
    const code = body?.error?.code ?? 'UNKNOWN_ERROR';
    const message = body?.error?.message ?? code;
    throw new ApiError(message, code, res.status);
  }

  return body.data as T;
}

export const api = {
  get<T>(url: string, init?: RequestInit): Promise<T> {
    return request<T>(url, { ...init, method: 'GET' });
  },

  post<T>(url: string, body?: unknown, init?: RequestInit): Promise<T> {
    return request<T>(url, {
      ...init,
      method: 'POST',
      headers: body !== undefined ? { 'Content-Type': 'application/json', ...init?.headers } : init?.headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  },

  patch<T>(url: string, body?: unknown, init?: RequestInit): Promise<T> {
    return request<T>(url, {
      ...init,
      method: 'PATCH',
      headers: body !== undefined ? { 'Content-Type': 'application/json', ...init?.headers } : init?.headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  },

  del<T>(url: string, init?: RequestInit): Promise<T> {
    return request<T>(url, { ...init, method: 'DELETE' });
  },
};
