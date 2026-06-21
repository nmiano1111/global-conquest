import { useEffect, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getUserByUsername, type UserRecord } from "../../api/users";
import { useAuth } from "../../auth";

export function ProfilePage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [profile, setProfile] = useState<UserRecord | null>(null);

  useEffect(() => {
    const username = auth.user?.username;
    if (!username) {
      setLoading(false);
      return;
    }

    let cancelled = false;
    const run = async () => {
      setLoading(true);
      setError("");
      try {
        const loaded = await getUserByUsername(username);
        if (!cancelled) setProfile(loaded);
      } catch (err) {
        if (cancelled) return;
        const apiErr = err as ApiError;
        if (apiErr.status === 401) {
          auth.clearSession();
          await navigate({ to: "/login" });
          return;
        }
        setError(apiErr.message || "Failed to load profile");
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    void run();
    return () => {
      cancelled = true;
    };
  }, [auth, navigate]);

  const username = profile?.username ?? auth.user?.username ?? "";
  const initials = username.slice(0, 2).toUpperCase();

  return (
    <div className="mx-auto max-w-lg">
      <h2 className="mb-4 text-xl font-semibold text-gc-text">Profile</h2>

      {loading ? (
        <div className="rounded-xl border border-gc-border bg-gc-surface p-6">
          <p className="text-sm text-gc-muted">Loading profile…</p>
        </div>
      ) : null}

      {error ? (
        <div className="rounded-xl border border-gc-danger/30 bg-gc-danger/10 p-4">
          <p className="text-sm text-gc-danger">{error}</p>
        </div>
      ) : null}

      {!loading && !error ? (
        <section className="rounded-xl border border-gc-border bg-gc-surface p-6">
          <div className="flex items-center gap-4 pb-5 border-b border-gc-border">
            <div
              className="flex h-14 w-14 shrink-0 items-center justify-center rounded-full bg-gc-surface-2 text-lg font-bold text-gc-accent"
              aria-hidden
            >
              {initials || "?"}
            </div>
            <div>
              <p className="font-semibold text-gc-text">{username || "—"}</p>
              <p className="text-xs text-gc-muted capitalize">{profile?.role ?? auth.user?.role ?? "player"}</p>
            </div>
          </div>

          <dl className="mt-5 grid gap-3 text-sm">
            <div className="flex items-center justify-between gap-4">
              <dt className="text-gc-muted">Username</dt>
              <dd className="font-medium text-gc-text">{profile?.username ?? auth.user?.username ?? "—"}</dd>
            </div>
            <div className="flex items-center justify-between gap-4">
              <dt className="text-gc-muted">User ID</dt>
              <dd className="font-mono text-xs text-gc-muted truncate max-w-[220px]">
                {profile?.id ?? auth.user?.id ?? "—"}
              </dd>
            </div>
            <div className="flex items-center justify-between gap-4">
              <dt className="text-gc-muted">Role</dt>
              <dd className="font-medium text-gc-text capitalize">{profile?.role ?? auth.user?.role ?? "—"}</dd>
            </div>
          </dl>
        </section>
      ) : null}
    </div>
  );
}
