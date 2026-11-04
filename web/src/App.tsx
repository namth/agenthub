import { useEffect, useState } from 'react';
import { Navigate, Route, Routes } from 'react-router';
import AppShell from './layout/AppShell';
import { ErrorBanner, Loading } from './components/StateView';
import { useAuth } from './hooks/useAuth';
import { visibleNav } from './nav';
import { api } from './api/client';
import Login from './pages/Login';
import Kanban from './pages/Kanban';
import TaskCreate from './pages/TaskCreate';
import TaskDetail from './pages/TaskDetail';
import Fleet from './pages/Fleet';
import LiveTerminal from './pages/LiveTerminal';
import Settings from './pages/Settings';
import Office from './pages/Office';

const PAGES: Record<string, React.ComponentType> = {
  'SCR-03': TaskCreate,
  'SCR-05': Fleet,
  'SCR-06': LiveTerminal,
  'SCR-08': Office,
};

export default function App() {
  const auth = useAuth();
  // null = Admin đang chờ đọc cài đặt: chưa dựng route để không chuyển hướng nhầm khỏi /office.
  const [officeSetting, setOfficeEnabled] = useState<boolean | null>(null);
  const isAdmin = auth.status === 'ready' && auth.user.role === 'ADMIN';
  const officeEnabled = officeSetting === true;

  useEffect(() => {
    if (isAdmin) {
      api.get<{ office_enabled: boolean }>('/api/admin/settings')
        .then((data) => setOfficeEnabled(Boolean(data?.office_enabled)))
        .catch(() => setOfficeEnabled(false));
    }
  }, [isAdmin]);

  if (auth.status === 'loading' || (isAdmin && officeSetting === null)) return <div className="p-6"><Loading /></div>;
  if (auth.status === 'error')
    return <div className="p-6"><ErrorBanner message={`Không tải được phiên đăng nhập: ${auth.message}`} onRetry={() => location.reload()} /></div>;
  if (auth.status === 'anonymous')
    return (
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    );

  return (
    <Routes>
      <Route element={<AppShell user={auth.user} officeEnabled={officeEnabled} />}>
        {visibleNav(auth.user.role, officeEnabled).map((n) => {
          if (n.code === 'SCR-02') {
            return <Route key={n.code} path={n.path} element={<Kanban role={auth.user.role} />} />;
          }
          if (n.code === 'SCR-04') {
            return <Route key={n.code} path={n.path} element={<TaskDetail role={auth.user.role} />} />;
          }
          if (n.code === 'SCR-07') {
            return <Route key={n.code} path={n.path} element={<Settings onOfficeChange={setOfficeEnabled} />} />;
          }
          const Page = PAGES[n.code];
          return <Route key={n.code} path={n.path} element={<Page />} />;
        })}
      </Route>
      <Route path="/login" element={<Navigate to="/" replace />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
