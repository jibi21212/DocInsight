"use client";

import { useEffect, useState } from "react";
import { X, Folder as FolderIcon, Inbox } from "lucide-react";
import { fetchFolders } from "@/store/app-store";
import type { Folder } from "@/lib/types";

interface FolderPickerProps {
  isOpen: boolean;
  currentFolderId: string | null | undefined;
  onSelect: (folderId: string | null) => void;
  onClose: () => void;
}

interface FlatFolder {
  folder: Folder;
  depth: number;
}

export function FolderPicker({
  isOpen,
  currentFolderId,
  onSelect,
  onClose,
}: FolderPickerProps) {
  const [flat, setFlat] = useState<FlatFolder[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      try {
        // BFS through hierarchy
        const result: FlatFolder[] = [];
        const queue: { parent: string | null; depth: number }[] = [
          { parent: null, depth: 0 },
        ];
        while (queue.length > 0) {
          const { parent, depth } = queue.shift()!;
          const res = await fetchFolders(parent);
          for (const f of res.folders) {
            result.push({ folder: f, depth });
            queue.push({ parent: f.id, depth: depth + 1 });
          }
        }
        if (!cancelled) setFlat(result);
      } catch (err) {
        console.error("Load folders failed:", err);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    load();
    return () => {
      cancelled = true;
    };
  }, [isOpen]);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={onClose}
    >
      <div
        className="w-full max-w-md rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-neutral-700 dark:bg-neutral-900"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-neutral-200 px-4 py-3 dark:border-neutral-700">
          <h3 className="text-sm font-semibold text-neutral-900 dark:text-white">
            Move to folder
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300"
          >
            <X size={16} />
          </button>
        </div>

        <div className="max-h-80 overflow-y-auto p-2">
          <button
            type="button"
            onClick={() => {
              onSelect(null);
              onClose();
            }}
            className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm ${
              currentFolderId == null
                ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
                : "text-neutral-700 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800"
            }`}
          >
            <Inbox size={14} />
            (Unfiled)
          </button>

          {loading ? (
            <div className="px-2 py-2 text-xs text-neutral-400">Loading…</div>
          ) : flat.length === 0 ? (
            <div className="px-2 py-2 text-xs text-neutral-400">
              No folders yet. Create one from the dashboard sidebar.
            </div>
          ) : (
            flat.map(({ folder, depth }) => (
              <button
                key={folder.id}
                type="button"
                onClick={() => {
                  onSelect(folder.id);
                  onClose();
                }}
                className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm ${
                  currentFolderId === folder.id
                    ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
                    : "text-neutral-700 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800"
                }`}
                style={{ paddingLeft: `${depth * 16 + 8}px` }}
              >
                <FolderIcon size={14} />
                <span className="truncate">{folder.name}</span>
              </button>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
