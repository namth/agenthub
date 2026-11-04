import { NavLink, Outlet } from 'react-router';
import { sidebarNav } from '../nav';
import type { User } from '../hooks/useAuth';

export default function AppShell({ user, officeEnabled }: { user: User; officeEnabled: boolean }) {
  return (
    <div className="flex min-h-screen">
      <aside className="w-56 shrink-0 border-r border-slate-200 bg-white px-3 py-4">
        <div className="mb-6 px-2 text-lg font-semibold">AgentHub</div>
        <nav className="flex flex-col gap-1">
          {sidebarNav(user.role, officeEnabled).map((n) => (
            <NavLink
              key={n.code}
              to={n.path}
              end={n.path === '/'}
              className={({ isActive }) =>
                `rounded-md px-2 py-1.5 text-sm ${isActive ? 'bg-slate-900 text-white' : 'text-slate-700 hover:bg-slate-100'}`
              }
            >
              {n.label}
            </NavLink>
          ))}
        </nav>
        <div className="mt-8 border-t border-slate-200 px-2 pt-3 text-xs text-slate-500">
          {user.username} · {user.role === 'ADMIN' ? 'Quản trị' : 'Đối tác'}
        </div>
      </aside>
      <main className="flex-1 p-6">
        <Outlet />
      </main>
    </div>
  );
}
