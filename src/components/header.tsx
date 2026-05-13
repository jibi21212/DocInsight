"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { FileText, Search, Upload, Moon, Sun, LogIn, LogOut } from "lucide-react";
import { useAppStore } from "@/store/app-store";
import { useAuth } from "@/lib/auth-context";
import { useEffect } from "react";

const navItems = [
  { href: "/", label: "Dashboard", icon: FileText },
  { href: "/upload", label: "Upload", icon: Upload },
  { href: "/search", label: "Search", icon: Search },
];

export function Header() {
  const pathname = usePathname();
  const { darkMode, toggleDarkMode } = useAppStore();
  const { user, logout } = useAuth();

  useEffect(() => {
    const stored = localStorage.getItem("darkMode");
    if (stored === "true") {
      document.documentElement.classList.add("dark");
      useAppStore.setState({ darkMode: true });
    }
  }, []);

  return (
    <header className="sticky top-0 z-50 border-b border-neutral-200 bg-white/80 backdrop-blur-sm dark:border-neutral-800 dark:bg-neutral-950/80">
      <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
        <Link href="/" className="flex items-center gap-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-600 text-white">
            <FileText size={18} />
          </div>
          <span className="text-lg font-semibold text-neutral-900 dark:text-white">
            DocInsight
          </span>
        </Link>

        <nav className="flex items-center gap-1">
          {navItems.map(({ href, label, icon: Icon }) => {
            const isActive =
              pathname === href ||
              (href !== "/" && pathname.startsWith(href));
            return (
              <Link
                key={href}
                href={href}
                className={`flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
                  isActive
                    ? "bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
                    : "text-neutral-600 hover:bg-neutral-100 hover:text-neutral-900 dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-white"
                }`}
              >
                <Icon size={16} />
                <span className="hidden sm:inline">{label}</span>
              </Link>
            );
          })}

          <button
            onClick={toggleDarkMode}
            className="ml-2 rounded-lg p-2 text-neutral-600 transition-colors hover:bg-neutral-100 dark:text-neutral-400 dark:hover:bg-neutral-800"
            aria-label="Toggle dark mode"
          >
            {darkMode ? <Sun size={18} /> : <Moon size={18} />}
          </button>

          {user ? (
            <div className="ml-2 flex items-center gap-2">
              <span className="hidden text-xs text-neutral-500 dark:text-neutral-400 sm:inline">
                {user.name || user.email}
              </span>
              <button
                onClick={logout}
                className="rounded-lg p-2 text-neutral-600 transition-colors hover:bg-neutral-100 dark:text-neutral-400 dark:hover:bg-neutral-800"
                aria-label="Sign out"
              >
                <LogOut size={16} />
              </button>
            </div>
          ) : (
            <Link
              href="/login"
              className="ml-2 flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm text-neutral-600 transition-colors hover:bg-neutral-100 dark:text-neutral-400 dark:hover:bg-neutral-800"
            >
              <LogIn size={16} />
              <span className="hidden sm:inline">Sign In</span>
            </Link>
          )}
        </nav>
      </div>
    </header>
  );
}
