"use client";

import Link from "next/link";
import { FileText, Globe, Trash2, Play, RefreshCcw, Calendar, HardDrive } from "lucide-react";
import { StatusBadge } from "./status-badge";
import type { Document } from "@/lib/types";

interface DocumentCardProps {
  document: Document;
  onProcess: (id: string) => void;
  onDelete: (id: string) => void;
  onRefresh?: (id: string) => void;
  processing?: boolean;
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function DocumentCard({
  document,
  onProcess,
  onDelete,
  onRefresh,
  processing,
}: DocumentCardProps) {
  return (
    <div className="group rounded-xl border border-neutral-200 bg-white p-5 shadow-sm transition-all hover:shadow-md dark:border-neutral-800 dark:bg-neutral-900">
      <div className="flex items-start justify-between gap-3">
        <Link
          href={`/documents/${document.id}`}
          className="flex min-w-0 flex-1 items-start gap-3"
        >
          <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${
            document.source_type === "web"
              ? "bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400"
              : "bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400"
          }`}>
            {document.source_type === "web" ? <Globe size={20} /> : <FileText size={20} />}
          </div>
          <div className="min-w-0 flex-1">
            <h3 className="truncate text-sm font-semibold text-neutral-900 group-hover:text-blue-600 dark:text-white dark:group-hover:text-blue-400">
              {document.name}
            </h3>
            <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400">
              <span className="flex items-center gap-1">
                <Calendar size={12} />
                {formatDate(document.created_at)}
              </span>
              <span className="flex items-center gap-1">
                <HardDrive size={12} />
                {formatFileSize(document.file_size)}
              </span>
              {document.page_count > 0 && (
                <span>{document.page_count} pages</span>
              )}
              {document.source_type === "web" && (
                <span className="rounded bg-emerald-100 px-1.5 py-0.5 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-400">
                  Web
                </span>
              )}
            </div>
            {document.source_url && (
              <p className="mt-1 truncate text-xs text-neutral-400 dark:text-neutral-500">
                {document.source_url}
              </p>
            )}
          </div>
        </Link>
        <StatusBadge status={document.status} />
      </div>

      {document.error_message && (
        <p className="mt-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
          {document.error_message}
        </p>
      )}

      <div className="mt-4 flex items-center gap-2 border-t border-neutral-100 pt-3 dark:border-neutral-800">
        {(document.status === "pending" || document.status === "failed") && (
          <button
            onClick={() => onProcess(document.id)}
            disabled={processing}
            className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
          >
            <Play size={12} />
            {document.status === "failed" ? "Retry" : "Process"}
          </button>
        )}
        {document.source_type === "web" && onRefresh && document.status !== "processing" && (
          <button
            onClick={() => onRefresh(document.id)}
            className="flex items-center gap-1.5 rounded-lg border border-emerald-200 px-3 py-1.5 text-xs font-medium text-emerald-600 transition-colors hover:bg-emerald-50 dark:border-emerald-800 dark:text-emerald-400 dark:hover:bg-emerald-900/20"
          >
            <RefreshCcw size={12} />
            Refresh
          </button>
        )}
        <button
          onClick={() => onDelete(document.id)}
          className="ml-auto flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium text-red-600 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
        >
          <Trash2 size={12} />
          Delete
        </button>
      </div>
    </div>
  );
}
