import { useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { login, signup } from "../../api/auth";
import { useAuth } from "../../auth";
import { buttonPrimaryClass, inputClass } from "./styles";

function AuthCard({ children }: { children: React.ReactNode }) {
  return (
    <main className="flex min-h-screen w-full items-center justify-center px-4 py-10">
      <div className="w-full max-w-sm">
        <div className="mb-8 text-center">
          <span
            className="text-2xl font-semibold tracking-widest text-gc-accent"
            style={{ fontFamily: "var(--font-display)" }}
          >
            Global Conquest
          </span>
        </div>
        <section className="rounded-2xl border border-gc-border bg-gc-surface p-6 shadow-xl shadow-black/40">
          {children}
        </section>
      </div>
    </main>
  );
}

export function LoginPage() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [form, setForm] = useState({ username: "", password: "" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const res = await login({
        username: form.username.trim(),
        password: form.password,
      });
      if (!res.token || !res.user.id || !res.user.username) {
        throw new Error("invalid login response from server");
      }
      auth.setSession(res.token, {
        id: res.user.id,
        username: res.user.username,
        role: res.user.role,
      });
      await navigate({ to: "/app/lobby" });
    } catch (err) {
      const apiErr = err as ApiError;
      setError(apiErr.message || "Login failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AuthCard>
      <h1 className="text-xl font-semibold text-gc-text">Welcome back</h1>
      <p className="mt-1 text-sm text-gc-muted">Sign in to continue.</p>

      <form className="mt-6 grid gap-4" onSubmit={onSubmit}>
        <label className="grid gap-1.5 text-sm font-medium text-gc-muted">
          Username
          <input
            className={inputClass}
            type="text"
            autoComplete="username"
            minLength={3}
            maxLength={24}
            value={form.username}
            onChange={(e) => setForm((prev) => ({ ...prev, username: e.target.value }))}
            required
          />
        </label>

        <label className="grid gap-1.5 text-sm font-medium text-gc-muted">
          Password
          <input
            className={inputClass}
            type="password"
            autoComplete="current-password"
            minLength={8}
            maxLength={128}
            value={form.password}
            onChange={(e) => setForm((prev) => ({ ...prev, password: e.target.value }))}
            required
          />
        </label>

        {error ? (
          <p className="rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">
            {error}
          </p>
        ) : null}

        <button className={buttonPrimaryClass} type="submit" disabled={submitting}>
          {submitting ? "Signing in…" : "Sign in"}
        </button>
      </form>

      <p className="mt-5 text-center text-sm text-gc-muted">
        Need an account?{" "}
        <Link className="font-medium text-gc-accent hover:text-gc-accent-dim transition-colors" to="/signup">
          Sign up
        </Link>
      </p>
    </AuthCard>
  );
}

export function SignupPage() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [form, setForm] = useState({
    username: "",
    password: "",
    confirmPassword: "",
  });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError("");
    if (form.password !== form.confirmPassword) {
      setError("Passwords do not match");
      return;
    }

    setSubmitting(true);
    try {
      await signup({
        username: form.username.trim(),
        password: form.password,
      });
      const loginRes = await login({
        username: form.username.trim(),
        password: form.password,
      });
      if (!loginRes.token || !loginRes.user.id || !loginRes.user.username) {
        throw new Error("invalid login response from server");
      }
      auth.setSession(loginRes.token, {
        id: loginRes.user.id,
        username: loginRes.user.username,
        role: loginRes.user.role,
      });
      await navigate({ to: "/app/lobby" });
    } catch (err) {
      const apiErr = err as ApiError;
      setError(apiErr.message || "Signup failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AuthCard>
      <h1 className="text-xl font-semibold text-gc-text">Create account</h1>
      <p className="mt-1 text-sm text-gc-muted">Set up your account to start playing.</p>

      <form className="mt-6 grid gap-4" onSubmit={onSubmit}>
        <label className="grid gap-1.5 text-sm font-medium text-gc-muted">
          Username
          <input
            className={inputClass}
            type="text"
            autoComplete="username"
            minLength={3}
            maxLength={24}
            value={form.username}
            onChange={(e) => setForm((prev) => ({ ...prev, username: e.target.value }))}
            required
          />
        </label>

        <label className="grid gap-1.5 text-sm font-medium text-gc-muted">
          Password
          <input
            className={inputClass}
            type="password"
            autoComplete="new-password"
            minLength={8}
            maxLength={128}
            value={form.password}
            onChange={(e) => setForm((prev) => ({ ...prev, password: e.target.value }))}
            required
          />
        </label>

        <label className="grid gap-1.5 text-sm font-medium text-gc-muted">
          Confirm Password
          <input
            className={inputClass}
            type="password"
            autoComplete="new-password"
            minLength={8}
            maxLength={128}
            value={form.confirmPassword}
            onChange={(e) => setForm((prev) => ({ ...prev, confirmPassword: e.target.value }))}
            required
          />
        </label>

        {error ? (
          <p className="rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">
            {error}
          </p>
        ) : null}

        <button className={buttonPrimaryClass} type="submit" disabled={submitting}>
          {submitting ? "Creating account…" : "Create account"}
        </button>
      </form>

      <p className="mt-5 text-center text-sm text-gc-muted">
        Already have an account?{" "}
        <Link className="font-medium text-gc-accent hover:text-gc-accent-dim transition-colors" to="/login">
          Sign in
        </Link>
      </p>
    </AuthCard>
  );
}
