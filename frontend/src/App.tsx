import { useEffect } from "react";
import { Routes, Route } from "react-router-dom";
import { Header } from "@/components/header";
import { Toaster } from "@/components/toast";
import { useAppStore } from "@/store/app-store";
import DashboardPage from "@/pages/DashboardPage";
import AddContentPage from "@/pages/AddContentPage";
import SearchPage from "@/pages/SearchPage";
import AgentPage from "@/pages/AgentPage";
import DocumentDetailPage from "@/pages/DocumentDetailPage";

export default function App() {
  useEffect(() => {
    if (localStorage.getItem("darkMode") === "true") {
      document.documentElement.classList.add("dark");
      useAppStore.setState({ darkMode: true });
    }
  }, []);

  return (
    <div className="flex min-h-full flex-col bg-background text-foreground">
      <Header />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-8 sm:px-6 lg:px-8">
        <Routes>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/upload" element={<AddContentPage />} />
          <Route path="/search" element={<SearchPage />} />
          <Route path="/agent" element={<AgentPage />} />
          <Route path="/documents/:id" element={<DocumentDetailPage />} />
        </Routes>
      </main>
      <Toaster />
      <footer className="border-t border-neutral-200 py-6 text-center text-xs text-neutral-500 dark:border-neutral-800 dark:text-neutral-400">
        DocInsight — Semantic Document Search &amp; Retrieval
      </footer>
    </div>
  );
}
