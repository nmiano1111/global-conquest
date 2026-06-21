import { Link, Outlet, useNavigate } from "@tanstack/react-router";
import { useAuth } from "../../auth";

export function AppShell() {
  const navigate = useNavigate();
  const auth = useAuth();

  const onLogout = async () => {
    auth.clearSession();
    await navigate({ to: "/login" });
  };

  const navLinkClass =
    "rounded-md px-3 py-1.5 text-sm font-medium text-gc-muted transition-colors hover:text-gc-text";
  const navLinkActiveClass = "!text-gc-text bg-gc-surface-2";

  return (
    <div className="mx-auto min-h-screen w-full max-w-[1700px] px-4 xl:px-6">
      <header className="sticky top-0 z-30 border-b border-gc-border bg-[#0c1118]/95 backdrop-blur-sm">
        <div className="flex h-14 items-center justify-between gap-4">
          <Link to="/app/lobby" className="shrink-0 select-none">
            <span
              className="text-lg font-semibold tracking-widest text-gc-accent"
              style={{ fontFamily: "var(--font-display)" }}
            >
              Global Conquest
            </span>
          </Link>

          <nav className="flex items-center gap-1">
            <Link
              className={navLinkClass}
              activeProps={{ className: navLinkActiveClass }}
              activeOptions={{ exact: false }}
              to="/app/lobby"
            >
              Lobby
            </Link>
            <Link
              className={navLinkClass}
              activeProps={{ className: navLinkActiveClass }}
              activeOptions={{ exact: false }}
              to="/app/profile"
            >
              Profile
            </Link>
            <Link
              className={navLinkClass}
              activeProps={{ className: navLinkActiveClass }}
              activeOptions={{ exact: false }}
              to="/app/leaderboard"
            >
              Leaderboard
            </Link>
            {auth.user?.role === "admin" ? (
              <Link
                className={navLinkClass}
                activeProps={{ className: navLinkActiveClass }}
                activeOptions={{ exact: false }}
                to="/app/admin"
              >
                Admin
              </Link>
            ) : null}
            <span className="mx-2 h-4 w-px bg-gc-border" aria-hidden />
            <button
              className="rounded-md px-3 py-1.5 text-sm font-medium text-gc-muted transition-colors hover:text-gc-danger"
              type="button"
              onClick={onLogout}
            >
              Sign out
            </button>
          </nav>
        </div>
      </header>

      <main className="py-6">
        <Outlet />
      </main>
    </div>
  );
}
