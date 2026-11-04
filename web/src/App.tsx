import { Navigate, Route, Routes } from 'react-router';
import AppShell from './layout/AppShell';
import { ErrorBanner, Loading } from './components/StateView';
import { useAuth } from './hooks/useAuth';
import { visibleNav } from './nav';
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
  'SCR-04': TaskDetail,
  'SCR-05': Fleet,
  'SCR-06': LiveTerminal,
  'SCR-07': Settings,
  'SCR-08': Office,
};

export default function App() {
  const auth = useAuth();
  // TODO(TASK-17): đọc office_enabled từ GET /api/admin/settings.
  const officeEnabled = false;

  if (auth.status === 'loading') return <div className="p-6"><Loading /></div>;
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
          const Page = PAGES[n.code];
          return <Route key={n.code} path={n.path} element={<Page />} />;
        })}
      </Route>
      <Route path="/login" element={<Navigate to="/" replace />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
