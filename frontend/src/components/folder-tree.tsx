import { useCallback, useEffect, useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  FolderPlus,
  Folder as FolderIcon,
  Trash2,
  Inbox,
} from "lucide-react";
import {
  fetchFolders,
  createFolder,
  deleteFolder,
} from "@/lib/api";
import type { Folder } from "@/lib/types";

interface FolderTreeProps {
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  refreshKey?: number;
}

interface NodeProps {
  folder: Folder;
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  onDelete: (id: string) => Promise<void>;
  depth: number;
}

function FolderNode({ folder, selectedId, onSelect, onDelete, depth }: NodeProps) {
  const [open, setOpen] = useState(true);
  const [children, setChildren] = useState<Folder[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");

  const loadChildren = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetchFolders(folder.id);
      setChildren(res.folders);
    } catch (err) {
      console.error("Load children failed:", err);
      setChildren([]);
    } finally {
      setLoading(false);
    }
  }, [folder.id]);

  useEffect(() => {
    if (open && children === null) {
      loadChildren();
    }
  }, [open, children, loadChildren]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) return;
    try {
      await createFolder(newName.trim(), folder.id);
      setNewName("");
      setCreating(false);
      setOpen(true);
      await loadChildren();
    } catch (err) {
      console.error("Create folder failed:", err);
      alert((err as Error).message);
    }
  };

  const isSelected = selectedId === folder.id;

  return (
    <div>
      <div
        className={`group flex items-center gap-1 rounded-md px-1.5 py-1 text-sm ${
          isSelected
            ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
            : "text-neutral-700 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800"
        }`}
        style={{ paddingLeft: `${depth * 12 + 6}px` }}
      >
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          className="text-neutral-500"
          aria-label={open ? "Collapse" : "Expand"}
        >
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
        <button
          type="button"
          onClick={() => onSelect(folder.id)}
          className="flex flex-1 items-center gap-1.5 truncate text-left"
        >
          <FolderIcon size={14} />
          <span className="truncate">{folder.name}</span>
        </button>
        <button
          type="button"
          onClick={() => setCreating((c) => !c)}
          className="opacity-0 transition-opacity hover:text-blue-600 group-hover:opacity-100"
          title="New subfolder"
        >
          <FolderPlus size={12} />
        </button>
        <button
          type="button"
          onClick={async () => {
            if (
              confirm(
                `Delete folder "${folder.name}"? Subfolders will be removed; documents become unfiled.`,
              )
            ) {
              await onDelete(folder.id);
            }
          }}
          className="opacity-0 transition-opacity hover:text-red-600 group-hover:opacity-100"
          title="Delete folder"
        >
          <Trash2 size={12} />
        </button>
      </div>

      {creating && (
        <form
          onSubmit={handleCreate}
          className="flex items-center gap-1 py-1"
          style={{ paddingLeft: `${(depth + 1) * 12 + 6}px` }}
        >
          <input
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="Folder name"
            autoFocus
            className="flex-1 rounded border border-neutral-200 px-2 py-1 text-xs dark:border-neutral-700 dark:bg-neutral-900 dark:text-white"
          />
          <button
            type="submit"
            className="rounded bg-blue-600 px-2 py-1 text-xs text-white"
          >
            Add
          </button>
          <button
            type="button"
            onClick={() => {
              setCreating(false);
              setNewName("");
            }}
            className="rounded px-2 py-1 text-xs text-neutral-500"
          >
            Cancel
          </button>
        </form>
      )}

      {open && (
        <div>
          {loading && (
            <div
              className="py-1 text-xs text-neutral-400"
              style={{ paddingLeft: `${(depth + 1) * 12 + 6}px` }}
            >
              Loading…
            </div>
          )}
          {children?.map((child) => (
            <FolderNode
              key={child.id}
              folder={child}
              selectedId={selectedId}
              onSelect={onSelect}
              onDelete={onDelete}
              depth={depth + 1}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function FolderTree({ selectedId, onSelect, refreshKey }: FolderTreeProps) {
  const [roots, setRoots] = useState<Folder[]>([]);
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [reload, setReload] = useState(0);

  const loadRoots = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetchFolders();
      setRoots(res.folders);
    } catch (err) {
      console.error("Load folders failed:", err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadRoots();
  }, [loadRoots, reload, refreshKey]);

  const handleDelete = async (id: string) => {
    try {
      await deleteFolder(id);
      if (selectedId === id) onSelect(null);
      setReload((r) => r + 1);
    } catch (err) {
      console.error("Delete folder failed:", err);
      alert((err as Error).message);
    }
  };

  const handleCreateRoot = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) return;
    try {
      await createFolder(newName.trim());
      setNewName("");
      setCreating(false);
      setReload((r) => r + 1);
    } catch (err) {
      console.error("Create folder failed:", err);
      alert((err as Error).message);
    }
  };

  return (
    <div className="space-y-1">
      <div className="mb-2 flex items-center justify-between px-1">
        <span className="text-xs font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
          Folders
        </span>
        <button
          type="button"
          onClick={() => setCreating((c) => !c)}
          className="text-neutral-500 hover:text-blue-600"
          title="New folder"
        >
          <FolderPlus size={14} />
        </button>
      </div>

      <button
        type="button"
        onClick={() => onSelect(null)}
        className={`flex w-full items-center gap-1.5 rounded-md px-2 py-1 text-sm ${
          selectedId === null
            ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
            : "text-neutral-700 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800"
        }`}
      >
        <Inbox size={14} />
        All documents
      </button>

      {creating && (
        <form onSubmit={handleCreateRoot} className="flex items-center gap-1 py-1">
          <input
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="Folder name"
            autoFocus
            className="flex-1 rounded border border-neutral-200 px-2 py-1 text-xs dark:border-neutral-700 dark:bg-neutral-900 dark:text-white"
          />
          <button
            type="submit"
            className="rounded bg-blue-600 px-2 py-1 text-xs text-white"
          >
            Add
          </button>
          <button
            type="button"
            onClick={() => {
              setCreating(false);
              setNewName("");
            }}
            className="rounded px-2 py-1 text-xs text-neutral-500"
          >
            Cancel
          </button>
        </form>
      )}

      {loading && roots.length === 0 ? (
        <div className="px-2 py-1 text-xs text-neutral-400">Loading…</div>
      ) : roots.length === 0 ? (
        <div className="px-2 py-1 text-xs text-neutral-400">No folders yet</div>
      ) : (
        roots.map((f) => (
          <FolderNode
            key={f.id}
            folder={f}
            selectedId={selectedId}
            onSelect={onSelect}
            onDelete={handleDelete}
            depth={0}
          />
        ))
      )}
    </div>
  );
}
