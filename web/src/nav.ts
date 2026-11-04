// Danh mục màn hình theo docs/03 mục 1 — nguồn duy nhất cho router và sidebar.
export type Role = 'ADMIN' | 'PARTNER';

export interface NavItem {
  code: string; // SCR-NN
  label: string;
  path: string;
  roles: Role[];
  inSidebar: boolean;
  requiresOffice?: boolean; // chỉ hiện khi bật văn phòng ảo (FEAT-02)
}

export const NAV: NavItem[] = [
  { code: 'SCR-02', label: 'Kanban', path: '/', roles: ['ADMIN', 'PARTNER'], inSidebar: true },
  { code: 'SCR-03', label: 'Tạo yêu cầu', path: '/tasks/new', roles: ['ADMIN', 'PARTNER'], inSidebar: true },
  { code: 'SCR-04', label: 'Chi tiết task', path: '/tasks/:id', roles: ['ADMIN', 'PARTNER'], inSidebar: false },
  { code: 'SCR-11', label: 'Máy của tôi', path: '/my/machines', roles: ['ADMIN', 'PARTNER'], inSidebar: true },
  { code: 'SCR-10', label: 'Đóng góp', path: '/contributions', roles: ['ADMIN', 'PARTNER'], inSidebar: true },
  { code: 'SCR-09', label: 'Dự án', path: '/projects', roles: ['ADMIN'], inSidebar: true },
  { code: 'SCR-05', label: 'Cụm máy', path: '/fleet', roles: ['ADMIN'], inSidebar: true },
  { code: 'SCR-06', label: 'Live terminal', path: '/fleet/:id/terminal', roles: ['ADMIN'], inSidebar: false },
  { code: 'SCR-08', label: 'Văn phòng', path: '/office', roles: ['ADMIN'], inSidebar: true, requiresOffice: true },
  { code: 'SCR-07', label: 'Cài đặt', path: '/settings', roles: ['ADMIN'], inSidebar: true },
];

export function visibleNav(role: Role, officeEnabled: boolean): NavItem[] {
  return NAV.filter((n) => n.roles.includes(role) && (!n.requiresOffice || officeEnabled));
}

export function sidebarNav(role: Role, officeEnabled: boolean): NavItem[] {
  return visibleNav(role, officeEnabled).filter((n) => n.inSidebar);
}
