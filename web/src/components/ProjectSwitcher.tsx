import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { ALL_PROJECTS, type Project, setSelectedProject, useSelectedProject } from '../project';

export default function ProjectSwitcher({ isOwner }: { isOwner: boolean }) {
  const selected = useSelectedProject();
  const [projects, setProjects] = useState<Project[] | null>(null);

  useEffect(() => {
    let alive = true;
    api
      .get<Project[]>('/api/projects')
      .then((data) => {
        if (alive) setProjects(data || []);
      })
      .catch(() => {
        if (alive) setProjects([]);
      });
    return () => {
      alive = false;
    };
  }, []);

  useEffect(() => {
    if (!projects || projects.length === 0) return;

    const isValid = (isOwner && selected === ALL_PROJECTS) || projects.some((p) => p.id === selected);
    if (!isValid) {
      const fallback = isOwner ? ALL_PROJECTS : projects[0].id;
      setSelectedProject(fallback);
    }
  }, [isOwner, projects, selected]);

  if (projects === null) {
    return <div className="text-xs text-slate-400">Đang tải...</div>;
  }

  if (projects.length === 0) {
    return <div className="px-2 py-1 text-xs text-slate-500">Chưa có dự án</div>;
  }

  const currentValue =
    (isOwner && selected === ALL_PROJECTS) || projects.some((p) => p.id === selected)
      ? (selected ?? (isOwner ? ALL_PROJECTS : projects[0].id))
      : (isOwner ? ALL_PROJECTS : projects[0].id);

  return (
    <div className="my-2">
      <select
        aria-label="Dự án"
        value={currentValue}
        onChange={(e) => setSelectedProject(e.target.value)}
        className="block w-full rounded-md border border-slate-300 bg-white px-2 py-1.5 text-xs text-slate-800 shadow-xs focus:border-slate-900 focus:outline-hidden focus:ring-1 focus:ring-slate-900"
      >
        {isOwner && <option value={ALL_PROJECTS}>Tất cả dự án</option>}
        {projects.map((p) => (
          <option key={p.id} value={p.id}>
            {p.github_repo}
          </option>
        ))}
      </select>
    </div>
  );
}
