import { useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { login, signup } from "../../api/auth";
import { useAuth } from "../../auth";
import { buttonPrimaryClass, inputClass } from "./styles";

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
    <main className="mx-auto flex min-h-screen w-full max-w-5xl items-center px-4 py-10">
      <section className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Welcome Back</h1>
        <p className="mt-1 text-sm text-slate-600">Sign in to continue to your lobby.</p>

        <form className="mt-6 grid gap-4" onSubmit={onSubmit}>
          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
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

          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
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

          {error ? <p className="text-sm text-rose-700">{error}</p> : null}

          <button className={buttonPrimaryClass} type="submit" disabled={submitting}>
            {submitting ? "Signing in..." : "Login"}
          </button>
        </form>

        <p className="mt-4 text-sm text-slate-600">
          Need an account?{" "}
          <Link className="font-medium text-slate-900 underline-offset-2 hover:underline" to="/signup">
            Sign up
          </Link>
        </p>
      </section>
    </main>
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
    <main className="mx-auto flex min-h-screen w-full max-w-5xl items-center px-4 py-10">
      <section className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Create Account</h1>
        <p className="mt-1 text-sm text-slate-600">Set up your account to start playing.</p>

        <form className="mt-6 grid gap-4" onSubmit={onSubmit}>
          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
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

          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
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

          <label className="grid gap-1.5 text-sm font-medium text-slate-700">
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

          {error ? <p className="text-sm text-rose-700">{error}</p> : null}

          <button className={buttonPrimaryClass} type="submit" disabled={submitting}>
            {submitting ? "Creating account..." : "Create account"}
          </button>
        </form>

        <p className="mt-4 text-sm text-slate-600">
          Already have an account?{" "}
          <Link className="font-medium text-slate-900 underline-offset-2 hover:underline" to="/login">
            Login
          </Link>
        </p>
      </section>
    </main>
  );
}
