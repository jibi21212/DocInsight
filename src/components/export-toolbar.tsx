"use client";

import { useState } from "react";
import { Download, Copy, Check } from "lucide-react";
import type { SearchResult } from "@/lib/types";
import {
  resultsToJSON,
  resultsToCSV,
  downloadBlob,
  copyToClipboard,
} from "@/lib/export-utils";

interface ExportToolbarProps {
  results: SearchResult[];
  query: string;
}

export function ExportToolbar({ results, query }: ExportToolbarProps) {
  const [copied, setCopied] = useState(false);

  if (results.length === 0) return null;

  const handleCopyJSON = async () => {
    const json = resultsToJSON(results, query);
    const ok = await copyToClipboard(json);
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleDownloadJSON = () => {
    const json = resultsToJSON(results, query);
    downloadBlob(json, `search-results-${Date.now()}.json`, "application/json");
  };

  const handleDownloadCSV = () => {
    const csv = resultsToCSV(results);
    downloadBlob(csv, `search-results-${Date.now()}.csv`, "text/csv");
  };

  return (
    <div className="flex items-center gap-2">
      <button
        onClick={handleCopyJSON}
        className="flex items-center gap-1.5 rounded-lg border border-neutral-200 px-2.5 py-1.5 text-xs font-medium text-neutral-600 transition-colors hover:bg-neutral-100 dark:border-neutral-700 dark:text-neutral-400 dark:hover:bg-neutral-800"
      >
        {copied ? <Check size={12} className="text-green-500" /> : <Copy size={12} />}
        {copied ? "Copied" : "Copy JSON"}
      </button>
      <button
        onClick={handleDownloadJSON}
        className="flex items-center gap-1.5 rounded-lg border border-neutral-200 px-2.5 py-1.5 text-xs font-medium text-neutral-600 transition-colors hover:bg-neutral-100 dark:border-neutral-700 dark:text-neutral-400 dark:hover:bg-neutral-800"
      >
        <Download size={12} />
        JSON
      </button>
      <button
        onClick={handleDownloadCSV}
        className="flex items-center gap-1.5 rounded-lg border border-neutral-200 px-2.5 py-1.5 text-xs font-medium text-neutral-600 transition-colors hover:bg-neutral-100 dark:border-neutral-700 dark:text-neutral-400 dark:hover:bg-neutral-800"
      >
        <Download size={12} />
        CSV
      </button>
    </div>
  );
}
