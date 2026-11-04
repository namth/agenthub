import { useState, useEffect } from 'react';
import { NavLink, Outlet } from 'react-router';
import { sidebarNav } from '../nav';
import type { User } from '../hooks/useAuth';
import ProjectSwitcher from '../components/ProjectSwitcher';

export default function AppShell({ user, officeEnabled }: { user: User; officeEnabled: boolean }) {
  const [isMenuOpen, setIsMenuOpen] = useState(false);

  useEffect(() => {
    if (!isMenuOpen) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setIsMenuOpen(false);
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isMenuOpen]);

  const navItems = sidebarNav(user.role, officeEnabled);

  return (
    <div className="flex min-h-screen flex-col md:flex-row">
      {/* Mobile top header */}
      <header
        data-testid="mobile-header"
        className="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3 md:hidden"
      >
        <span className="text-lg font-semibold text-slate-900">AgentHub</span>
        <button
          type="button"
          aria-label="Mở menu"
          aria-expanded={isMenuOpen}
          onClick={() => setIsMenuOpen((prev) => !prev)}
          className="rounded-lg p-2 text-slate-600 hover:bg-slate-100"
        >
          <svg className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
          </svg>
        </button>
      </header>

      {/* Mobile drawer dialog */}
      {isMenuOpen && (
        <>
          <div
            data-testid="menu-backdrop"
            onClick={() => setIsMenuOpen(false)}
            className="fixed inset-0 z-40 bg-slate-900/50 backdrop-blur-xs md:hidden"
          />
          <div
            role="dialog"
            aria-label="Menu"
            className="fixed inset-y-0 left-0 z-50 flex w-72 flex-col bg-white p-4 shadow-xl md:hidden"
          >
            <div className="mb-4 flex items-center justify-between px-2">
              <span className="text-lg font-semibold text-slate-900">AgentHub</span>
              <button
                type="button"
                onClick={() => setIsMenuOpen(false)}
                className="rounded-md p-1.5 text-slate-500 hover:bg-slate-100"
              >
                ✕
              </button>
            </div>
            <div className="mb-4 px-1">
              <ProjectSwitcher isOwner={user.role === 'ADMIN'} />
            </div>
            <nav className="flex flex-1 flex-col gap-1">
              {navItems.map((n) => (
                <NavLink
                  key={n.code}
                  to={n.path}
                  end={n.path === '/'}
                  onClick={() => setIsMenuOpen(false)}
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
          </div>
        </>
      )}

      {/* Desktop sidebar */}
      <aside
        data-testid="sidebar"
        className="hidden md:flex w-56 shrink-0 flex-col border-r border-slate-200 bg-white px-3 py-4"
      >
        <div className="mb-4 px-2 text-lg font-semibold">AgentHub</div>
        <div className="mb-4 px-1">
          <ProjectSwitcher isOwner={user.role === 'ADMIN'} />
        </div>
        <nav className="flex flex-col gap-1">
          {navItems.map((n) => (
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

      {/* Main content */}
      <main className="flex-1 min-w-0 p-4 md:p-6">
        <Outlet />
      </main>
    </div>
  );
}
