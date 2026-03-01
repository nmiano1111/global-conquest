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

  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
      <h2 className="text-2xl font-semibold tracking-tight text-slate-900">Profile</h2>
      {loading ? <p className="mt-3 text-sm text-slate-600">Loading profile...</p> : null}
      {error ? <p className="mt-3 text-sm text-rose-700">{error}</p> : null}
      {!loading && !error ? (
        <dl className="mt-4 grid gap-2 text-sm text-slate-700">
          <div className="grid grid-cols-[100px_1fr] gap-2">
            <dt className="text-slate-500">Username</dt>
            <dd className="font-medium text-slate-900">{profile?.username ?? auth.user?.username ?? "-"}</dd>
          </div>
          <div className="grid grid-cols-[100px_1fr] gap-2">
            <dt className="text-slate-500">User ID</dt>
            <dd className="font-mono text-xs text-slate-700">{profile?.id ?? auth.user?.id ?? "-"}</dd>
          </div>
          <div className="grid grid-cols-[100px_1fr] gap-2">
            <dt className="text-slate-500">Role</dt>
            <dd className="font-medium text-slate-900">{profile?.role ?? auth.user?.role ?? "-"}</dd>
          </div>
        </dl>
      ) : null}
    </section>
  );
}
