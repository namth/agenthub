# web — Dashboard AgentHub (Vite + React + Tailwind)

```bash
npm install
npm run dev        # http://localhost:5173, proxy /api và /ws sang server Go :8080
npm test           # vitest
npm run build      # ra web/dist, được Go nhúng qua web/embed.go
```

- Danh mục màn hình và phân quyền điều hướng: `src/nav.ts` (khớp `docs/03` mục 1).
- 4 trạng thái UI dùng chung: `src/components/StateView.tsx`.
- Trang trong `src/pages/` hiện là khung tạm; nội dung thật làm theo task trong `docs/11`.
