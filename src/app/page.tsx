"use client";

import { useEffect, useState, useCallback } from "react";
import { FileText, Search, Upload, TrendingUp } from "lucide-react";
import Link from "next/link";
import { DocumentCard } from "@/components/document-card";
import { SearchBar } from "@/components/search-bar";
import { EmptyState } from "@/components/empty-state";
import { FolderTree } from "@/components/folder-tree";
import { useAppStore } from "@/store/app-store";
import {
  fetchDocuments,
  processDocument,
  deleteDocument,
  refreshDocument,
  searchDocuments,
} from "@/store/app-store";
import { SearchResultCard } from "@/components/search-result-card";
import { ExportToolbar } from "@/components/export-toolbar";
import type { SearchResult, SearchMode } from "@/lib/types";

export default function DashboardPage() {
  const {
    documents,
    documentsLoading,
    totalDocuments,
    setDocuments,
    setDocumentsLoading,
    updateDocumentStatus,
    removeDocument,
    selectedFolderId,
    setSelectedFolderId,
  } = useAppStore();

  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchLoading, setSearchLoading] = useState(false);
  const [searchTookMs, setSearchTookMs] = useState(0);
  const [processingIds, setProcessingIds] = useState<Set<string>>(new Set());

  const loadDocuments = useCallback(async () => {
    setDocumentsLoading(true);
    try {
      const res = await fetchDocuments(1, 12, selectedFolderId);
      setDocuments(res.data, res.total);
    } catch (err) {
      console.error("Failed to load documents:", err);
    } finally {
      setDocumentsLoading(false);
    }
  }, [setDocuments, setDocumentsLoading, selectedFolderId]);

  useEffect(() => {
    loadDocuments();
  }, [loadDocuments]);

  // Poll for status updates on processing documents
  useEffect(() => {
    const hasProcessing = documents.some(
      (d) => d.status === "processing" || processingIds.has(d.id)
    );
    if (!hasProcessing) return;

    const interval = setInterval(loadDocuments, 3000);
    return () => clearInterval(interval);
  }, [documents, processingIds, loadDocuments]);

  const handleProcess = async (id: string) => {
    setProcessingIds((prev) => new Set(prev).add(id));
    updateDocumentStatus(id, "processing");
    try {
      await processDocument(id);
    } catch (err) {
      console.error("Process failed:", err);
      updateDocumentStatus(id, "failed", String(err));
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await deleteDocument(id);
      removeDocument(id);
    } catch (err) {
      console.error("Delete failed:", err);
    }
  };

  const handleRefresh = async (id: string) => {
    updateDocumentStatus(id, "processing");
    try {
      await refreshDocument(id);
    } catch (err) {
      console.error("Refresh failed:", err);
      updateDocumentStatus(id, "failed", String(err));
    }
  };

  const handleSearch = async (
    query: string,
    topK: number,
    threshold: number,
    searchMode?: SearchMode
  ) => {
    setSearchLoading(true);
    setSearchQuery(query);
    try {
      const res = await searchDocuments(
        query,
        topK,
        threshold,
        undefined,
        searchMode,
        selectedFolderId,
      );
      setSearchResults(res.results);
      setSearchTookMs(res.took_ms);
    } catch (err) {
      console.error("Search failed:", err);
    } finally {
      setSearchLoading(false);
    }
  };

  const completedCount = documents.filter(
    (d) => d.status === "completed"
  ).length;
  const processingCount = documents.filter(
    (d) => d.status === "processing"
  ).length;

  return (
    <div className="flex flex-col gap-6 lg:flex-row">
      <aside className="w-full shrink-0 rounded-xl border border-neutral-200 bg-white p-3 dark:border-neutral-800 dark:bg-neutral-900 lg:w-60">
        <FolderTree
          selectedId={selectedFolderId}
          onSelect={setSelectedFolderId}
        />
      </aside>
      <div className="min-w-0 flex-1 space-y-8">
      {/* Stats */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="rounded-xl border border-neutral-200 bg-white p-5 dark:border-neutral-800 dark:bg-neutral-900">
          <div className="flex items-center gap-3">
            <div className="rounded-lg bg-blue-50 p-2.5 dark:bg-blue-900/30">
              <FileText size={20} className="text-blue-600 dark:text-blue-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-neutral-900 dark:text-white">
                {totalDocuments}
              </p>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                Total Documents
              </p>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-neutral-200 bg-white p-5 dark:border-neutral-800 dark:bg-neutral-900">
          <div className="flex items-center gap-3">
            <div className="rounded-lg bg-green-50 p-2.5 dark:bg-green-900/30">
              <TrendingUp
                size={20}
                className="text-green-600 dark:text-green-400"
              />
            </div>
            <div>
              <p className="text-2xl font-bold text-neutral-900 dark:text-white">
                {completedCount}
              </p>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                Ready for Search
              </p>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-neutral-200 bg-white p-5 dark:border-neutral-800 dark:bg-neutral-900">
          <div className="flex items-center gap-3">
            <div className="rounded-lg bg-purple-50 p-2.5 dark:bg-purple-900/30">
              <Search
                size={20}
                className="text-purple-600 dark:text-purple-400"
              />
            </div>
            <div>
              <p className="text-2xl font-bold text-neutral-900 dark:text-white">
                {processingCount}
              </p>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                Processing
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Search */}
      <section>
        <h2 className="mb-4 text-lg font-semibold text-neutral-900 dark:text-white">
          Semantic Search
        </h2>
        <SearchBar onSearch={handleSearch} loading={searchLoading} />

        {searchResults.length > 0 && (
          <div className="mt-6 space-y-4">
            <div className="flex items-center justify-between">
              <p className="text-sm text-neutral-500 dark:text-neutral-400">
                {searchResults.length} results in {searchTookMs}ms
              </p>
              <div className="flex items-center gap-3">
                <ExportToolbar results={searchResults} query={searchQuery} />
                <button
                  onClick={() => {
                    setSearchResults([]);
                    setSearchQuery("");
                  }}
                  className="text-xs text-blue-600 hover:underline dark:text-blue-400"
                >
                  Clear results
                </button>
              </div>
            </div>
            {searchResults.map((result, i) => (
              <SearchResultCard
                key={result.chunk_id}
                result={result}
                index={i}
                query={searchQuery}
              />
            ))}
          </div>
        )}
      </section>

      {/* Recent Documents */}
      <section>
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-white">
            Recent Documents
          </h2>
          <Link
            href="/upload"
            className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-2 text-xs font-medium text-white transition-colors hover:bg-blue-700"
          >
            <Upload size={14} />
            Upload New
          </Link>
        </div>

        {documentsLoading && documents.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-8 w-8 animate-spin rounded-full border-2 border-blue-600 border-t-transparent" />
          </div>
        ) : documents.length === 0 ? (
          <EmptyState
            icon={FileText}
            title="No documents yet"
            description="Upload a PDF or add web page URLs to get started with semantic search."
            actionLabel="Add Content"
            actionHref="/upload"
          />
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            {documents.map((doc) => (
              <DocumentCard
                key={doc.id}
                document={doc}
                onProcess={handleProcess}
                onDelete={handleDelete}
                onRefresh={handleRefresh}
                onMoved={loadDocuments}
                processing={processingIds.has(doc.id)}
              />
            ))}
          </div>
        )}
      </section>
      </div>
    </div>
  );
}
