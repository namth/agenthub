import { useEffect, useState } from 'react';

export const PROJECT_KEY = 'agenthub.project';
export const ALL_PROJECTS = '*';

export interface Project {
  id: string;
  name: string;
  github_repo: string;
  default_branch: string;
  status: 'ACTIVE' | 'ARCHIVED' | string;
  role?: 'ADMIN' | 'PARTNER' | string;
  weight: number;
  max_parallel: number | null;
  webhook_ok: boolean;
  open_tasks: number;
}

export function getSelectedProject(): string | null {
  try {
    return localStorage.getItem(PROJECT_KEY);
  } catch {
    return null;
  }
}

export function setSelectedProject(id: string): void {
  try {
    localStorage.setItem(PROJECT_KEY, id);
  } catch {
    // Bỏ qua lỗi localStorage nếu bị chặn hoặc quota
  }
  window.dispatchEvent(new Event('agenthub:project'));
}

export function useSelectedProject(): string | null {
  const [selected, setSelected] = useState<string | null>(getSelectedProject);

  useEffect(() => {
    const update = () => {
      setSelected(getSelectedProject());
    };

    window.addEventListener('agenthub:project', update);
    window.addEventListener('storage', update);
    return () => {
      window.removeEventListener('agenthub:project', update);
      window.removeEventListener('storage', update);
    };
  }, []);

  return selected;
}
