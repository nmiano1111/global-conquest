import { Link, Outlet, useNavigate } from "@tanstack/react-router";
import { useAuth } from "../../auth";
import { buttonGhostClass } from "./styles";

export function AppShell() {
  const navigate = useNavigate();
  const auth = useAuth();

  const onLogout = async () => {
    auth.clearSession();
    await navigate({ to: "/login" });
  };

  return (
    <main className="mx-auto min-h-screen w-full max-w-[1700px] px-4 py-8 xl:px-6">
      <header className="rounded-2xl border border-slate-200 bg-white px-5 py-4 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-lg font-semibold tracking-tight text-slate-900">Global Conquest</h1>
          <nav className="flex items-center gap-2">
            <Link
              className="rounded-lg px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
              to="/app/lobby"
            >
              Lobby
            </Link>
            <Link
              className="rounded-lg px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
              to="/app/profile"
            >
              Profile
            </Link>
            {auth.user?.role === "admin" ? (
              <Link
                className="rounded-lg px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
                to="/app/admin"
              >
                Admin
              </Link>
            ) : null}
            <button className={buttonGhostClass} type="button" onClick={onLogout}>
              Logout
            </button>
          </nav>
        </div>
      </header>

      <section className="mt-4">
        <Outlet />
      </section>
    </main>
  );
}
