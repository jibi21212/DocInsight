"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { AlertCircle, Key, UserPlus } from "lucide-react";
import { useAuth } from "@/lib/auth-context";

type Mode = "login" | "register";

export default function LoginPage() {
  const router = useRouter();
  const { login, register } = useAuth();
  const [mode, setMode] = useState<Mode>("login");
  const [apiKey, setApiKey] = useState("");
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [generatedKey, setGeneratedKey] = useState<string | null>(null);

  const handleLogin = async () => {
    setLoading(true);
    setError(null);
    try {
      await login(apiKey);
      router.push("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setLoading(false);
    }
  };

  const handleRegister = async () => {
    setLoading(true);
    setError(null);
    try {
      const key = await register(email, name);
      setGeneratedKey(key);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Registration failed");
    } finally {
      setLoading(false);
    }
  };

  if (generatedKey) {
    return (
      <div className="mx-auto max-w-md space-y-6 py-12">
        <div className="rounded-xl border border-green-200 bg-green-50 p-6 dark:border-green-800 dark:bg-green-900/20">
          <h2 className="text-lg font-bold text-green-800 dark:text-green-300">
            Registration Successful
          </h2>
          <p className="mt-2 text-sm text-green-700 dark:text-green-400">
            Save your API key below. It will not be shown again.
          </p>
          <code className="mt-3 block break-all rounded bg-green-100 p-3 text-sm font-mono text-green-900 dark:bg-green-800/50 dark:text-green-200">
            {generatedKey}
          </code>
          <button
            onClick={() => router.push("/")}
            className="mt-4 w-full rounded-lg bg-green-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-green-700"
          >
            Continue to Dashboard
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-md space-y-6 py-12">
      <div>
        <h1 className="text-2xl font-bold text-neutral-900 dark:text-white">
          {mode === "login" ? "Sign In" : "Create Account"}
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          {mode === "login"
            ? "Enter your API key to access your documents."
            : "Register to get an API key for DocInsight."}
        </p>
      </div>

      <div className="flex rounded-lg border border-neutral-200 bg-neutral-50 p-1 dark:border-neutral-800 dark:bg-neutral-900">
        <button
          onClick={() => setMode("login")}
          className={`flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all ${
            mode === "login"
              ? "bg-white text-neutral-900 shadow-sm dark:bg-neutral-800 dark:text-white"
              : "text-neutral-500 dark:text-neutral-400"
          }`}
        >
          <Key size={16} />
          Sign In
        </button>
        <button
          onClick={() => setMode("register")}
          className={`flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all ${
            mode === "register"
              ? "bg-white text-neutral-900 shadow-sm dark:bg-neutral-800 dark:text-white"
              : "text-neutral-500 dark:text-neutral-400"
          }`}
        >
          <UserPlus size={16} />
          Register
        </button>
      </div>

      {mode === "login" ? (
        <div className="space-y-4">
          <div>
            <label className="mb-2 block text-sm font-medium text-neutral-700 dark:text-neutral-300">
              API Key
            </label>
            <input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder="di_..."
              className="w-full rounded-xl border border-neutral-300 bg-white px-4 py-3 text-sm text-neutral-900 placeholder-neutral-400 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 dark:border-neutral-700 dark:bg-neutral-900 dark:text-white"
            />
          </div>
          <button
            onClick={handleLogin}
            disabled={loading || !apiKey.trim()}
            className="w-full rounded-lg bg-blue-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {loading ? "Signing in..." : "Sign In"}
          </button>
        </div>
      ) : (
        <div className="space-y-4">
          <div>
            <label className="mb-2 block text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Email
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              className="w-full rounded-xl border border-neutral-300 bg-white px-4 py-3 text-sm text-neutral-900 placeholder-neutral-400 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 dark:border-neutral-700 dark:bg-neutral-900 dark:text-white"
            />
          </div>
          <div>
            <label className="mb-2 block text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Name
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Your name"
              className="w-full rounded-xl border border-neutral-300 bg-white px-4 py-3 text-sm text-neutral-900 placeholder-neutral-400 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 dark:border-neutral-700 dark:bg-neutral-900 dark:text-white"
            />
          </div>
          <button
            onClick={handleRegister}
            disabled={loading || !email.trim()}
            className="w-full rounded-lg bg-blue-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {loading ? "Creating account..." : "Register"}
          </button>
        </div>
      )}

      {error && (
        <div className="flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
          <AlertCircle size={16} className="shrink-0" />
          {error}
        </div>
      )}
    </div>
  );
}
