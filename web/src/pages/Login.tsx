// SCR-01 — đăng nhập GitHub OAuth (docs/07 mục 1.1).
export default function Login() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <div className="w-80 space-y-6 rounded-xl border border-slate-200 bg-white p-8 text-center shadow-sm">
        <h1 className="text-2xl font-semibold">AgentHub</h1>
        <a href="/api/auth/github" className="block rounded-md bg-slate-900 px-4 py-2 text-white hover:bg-slate-800">
          Đăng nhập bằng GitHub
        </a>
      </div>
    </div>
  );
}
