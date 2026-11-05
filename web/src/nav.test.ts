import { sidebarNav, visibleNav } from './nav';

describe('phân quyền điều hướng (docs/02, docs/03)', () => {
  it('Partner không thấy Fleet, Terminal, Cài đặt, Dự án; thấy Văn phòng khi bật (DEC-27)', () => {
    expect(visibleNav('PARTNER', true).map((n) => n.code)).toEqual(['SCR-02', 'SCR-03', 'SCR-04', 'SCR-11', 'SCR-10', 'SCR-08', 'SCR-12']); // SCR-12 Hồ sơ (TASK-046)
    expect(visibleNav('PARTNER', false).map((n) => n.code)).toEqual(['SCR-02', 'SCR-03', 'SCR-04', 'SCR-11', 'SCR-10', 'SCR-12']);
  });

  it('Admin thấy Văn phòng chỉ khi bật', () => {
    expect(sidebarNav('ADMIN', false).map((n) => n.code)).not.toContain('SCR-08');
    expect(sidebarNav('ADMIN', true).map((n) => n.code)).toContain('SCR-08');
  });

  it('sidebar không chứa route có tham số', () => {
    for (const n of sidebarNav('ADMIN', true)) expect(n.path).not.toContain(':');
  });
});
