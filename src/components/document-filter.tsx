"use client";

import { useEffect, useState } from "react";
import { Filter, X } from "lucide-react";
import { fetchDocuments } from "@/store/app-store";
import type { Document } from "@/lib/types";

interface DocumentFilterProps {
  selectedIds: string[];
  onChange: (ids: string[]) => void;
}

export function DocumentFilter({ selectedIds, onChange }: DocumentFilterProps) {
  const [documents, setDocuments] = useState<Document[]>([]);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    fetchDocuments(1, 100)
      .then((res) => setDocuments(res.data.filter((d) => d.status === "completed")))
      .catch(() => {});
  }, []);

  const toggle = (id: string) => {
    if (selectedIds.includes(id)) {
      onChange(selectedIds.filter((i) => i !== id));
    } else {
      onChange([...selectedIds, id]);
    }
  };

  if (documents.length === 0) return null;

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-colors ${
          selectedIds.length > 0
            ? "border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-500 dark:bg-blue-900/20 dark:text-blue-400"
            : "border-neutral-200 text-neutral-600 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-400 dark:hover:bg-neutral-800"
        }`}
      >
        <Filter size={14} />
        Filter by document
        {selectedIds.length > 0 && (
          <span className="rounded-full bg-blue-600 px-1.5 py-0.5 text-[10px] text-white">
            {selectedIds.length}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute left-0 top-full z-10 mt-2 w-72 rounded-xl border border-neutral-200 bg-white p-2 shadow-lg dark:border-neutral-700 dark:bg-neutral-900">
          <div className="mb-2 flex items-center justify-between px-2">
            <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">
              Select documents
            </span>
            {selectedIds.length > 0 && (
              <button
                onClick={() => onChange([])}
                className="flex items-center gap-1 text-xs text-red-500 hover:text-red-600"
              >
                <X size={12} />
                Clear
              </button>
            )}
          </div>
          <div className="max-h-48 space-y-1 overflow-y-auto">
            {documents.map((doc) => (
              <label
                key={doc.id}
                className="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800"
              >
                <input
                  type="checkbox"
                  checked={selectedIds.includes(doc.id)}
                  onChange={() => toggle(doc.id)}
                  className="rounded border-neutral-300 text-blue-600 focus:ring-blue-500"
                />
                <span className="truncate text-neutral-700 dark:text-neutral-300">
                  {doc.name}
                </span>
              </label>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
