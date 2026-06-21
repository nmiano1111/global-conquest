import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { listAdminUsers, revokeUserSessions, updateUserAccess, type AdminUserRecord } from "../../api/users";
import { useAuth } from "../../auth";
import { buttonGhostClass, buttonPrimaryClass } from "./styles";

export function AdminPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [users, setUsers] = useState<AdminUserRecord[]>([]);
  const [busyUserID, setBusyUserID] = useState("");

  const load = useCallback(
    async (cancelled = false) => {
      setError("");
      try {
        const out = await listAdminUsers();
        if (cancelled) return;
        setUsers(out);
      } catch (err) {
        if (cancelled) return;
        const apiErr = err as ApiError;
        if (apiErr.status === 401) {
          auth.clearSession();
          await navigate({ to: "/login" });
          return;
        }
        if (apiErr.status === 403) {
          await navigate({ to: "/app/lobby" });
          return;
        }
        setError(apiErr.message || "Failed to load admin users");
      }
    },
    [auth, navigate]
  );

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      setLoading(true);
      await load(cancelled);
      if (!cancelled) setLoading(false);
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [load]);

  const patchAccess = async (u: AdminUserRecord, accessStatus: "active" | "blocked") => {
    setBusyUserID(u.id);
    setError("");
    try {
      await updateUserAccess(u.id, accessStatus);
      await load();
    } catch (err) {
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      if (apiErr.status === 403) {
        await navigate({ to: "/app/lobby" });
        return;
      }
      setError(apiErr.message || "Failed to update user access");
    } finally {
      setBusyUserID("");
    }
  };

  const onRevokeSessions = async (u: AdminUserRecord) => {
    setBusyUserID(u.id);
    setError("");
    try {
      await revokeUserSessions(u.id);
      await load();
    } catch (err) {
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      if (apiErr.status === 403) {
        await navigate({ to: "/app/lobby" });
        return;
      }
      setError(apiErr.message || "Failed to revoke sessions");
    } finally {
      setBusyUserID("");
    }
  };

  return (
    <div>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-gc-text">Admin</h2>
          <p className="mt-0.5 text-sm text-gc-muted">Manage user access and sessions.</p>
        </div>
        <button
          className={buttonGhostClass}
          type="button"
          onClick={() => void load()}
          disabled={loading || !!busyUserID}
        >
          Refresh
        </button>
      </div>

      {loading ? <p className="text-sm text-gc-muted">Loading users…</p> : null}
      {error ? (
        <p className="mb-4 rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">
          {error}
        </p>
      ) : null}

      <section className="overflow-x-auto rounded-xl border border-gc-border bg-gc-surface">
        <table className="min-w-full divide-y divide-gc-border text-sm">
          <thead className="bg-gc-surface-2">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">User</th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Role</th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Status</th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Sessions</th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gc-border/60">
            {users.map((u) => {
              const isBusy = busyUserID === u.id;
              const isBlocked = u.accessStatus === "blocked";
              return (
                <tr key={u.id} className="transition-colors hover:bg-gc-surface-2">
                  <td className="px-4 py-3">
                    <p className="font-medium text-gc-text">{u.username}</p>
                    <p className="font-mono text-[11px] text-gc-muted">{u.id}</p>
                  </td>
                  <td className="px-4 py-3 capitalize text-gc-muted">{u.role}</td>
                  <td className="px-4 py-3">
                    <span
                      className={`inline-flex rounded-full px-2 py-0.5 text-[11px] font-medium ${
                        isBlocked
                          ? "border border-gc-danger/40 bg-gc-danger/10 text-gc-danger"
                          : "border border-gc-success/40 bg-gc-success/10 text-gc-success"
                      }`}
                    >
                      {isBlocked ? "Blocked" : "Active"}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-gc-muted">{u.activeSessions}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-2">
                      {isBlocked ? (
                        <button
                          className={buttonGhostClass}
                          type="button"
                          disabled={isBusy}
                          onClick={() => void patchAccess(u, "active")}
                        >
                          Unblock
                        </button>
                      ) : (
                        <button
                          className={buttonGhostClass}
                          type="button"
                          disabled={isBusy}
                          onClick={() => void patchAccess(u, "blocked")}
                        >
                          Block
                        </button>
                      )}
                      <button
                        className={buttonPrimaryClass}
                        type="button"
                        disabled={isBusy}
                        onClick={() => void onRevokeSessions(u)}
                      >
                        Revoke Sessions
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </section>
    </div>
  );
}
