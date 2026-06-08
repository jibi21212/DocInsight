import { useState, useEffect, useRef } from "react";
import { Plus, Tag as TagIcon } from "lucide-react";
import { TagBadge } from "./tag-badge";
import {
  fetchTags,
  createTag,
  addTagToDocument,
  removeTagFromDocument,
} from "@/lib/api";
import type { Tag } from "@/lib/types";

const TAG_COLORS = [
  "#6366f1", "#ec4899", "#f59e0b", "#10b981",
  "#3b82f6", "#ef4444", "#8b5cf6", "#14b8a6",
];

interface TagManagerProps {
  documentId: string;
  documentTags: Tag[];
  onTagsChange: (tags: Tag[]) => void;
}

export function TagManager({
  documentId,
  documentTags,
  onTagsChange,
}: TagManagerProps) {
  const [allTags, setAllTags] = useState<Tag[]>([]);
  const [open, setOpen] = useState(false);
  const [newTagName, setNewTagName] = useState("");
  const [creating, setCreating] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (open) {
      fetchTags()
        .then((res) => setAllTags(res.tags))
        .catch(console.error);
    }
  }, [open]);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const handleAdd = async (tagId: string) => {
    try {
      const res = await addTagToDocument(documentId, tagId);
      onTagsChange(res.tags);
    } catch (err) {
      console.error("Failed to add tag:", err);
    }
  };

  const handleRemove = async (tagId: string) => {
    try {
      const res = await removeTagFromDocument(documentId, tagId);
      onTagsChange(res.tags);
    } catch (err) {
      console.error("Failed to remove tag:", err);
    }
  };

  const handleCreate = async () => {
    if (!newTagName.trim()) return;
    setCreating(true);
    try {
      const color = TAG_COLORS[allTags.length % TAG_COLORS.length];
      const res = await createTag(newTagName.trim(), color);
      setAllTags((prev) => [...prev, res.tag]);
      await handleAdd(res.tag.id);
      setNewTagName("");
    } catch (err) {
      console.error("Failed to create tag:", err);
    } finally {
      setCreating(false);
    }
  };

  const docTagIds = new Set(documentTags.map((t) => t.id));
  const availableTags = allTags.filter((t) => !docTagIds.has(t.id));

  return (
    <div ref={ref} className="relative">
      <div className="flex flex-wrap items-center gap-1.5">
        {documentTags.map((tag) => (
          <TagBadge key={tag.id} tag={tag} onRemove={handleRemove} />
        ))}
        <button
          onClick={() => setOpen(!open)}
          className="flex items-center gap-1 rounded-full border border-dashed border-neutral-300 px-2 py-0.5 text-xs text-neutral-500 transition-colors hover:border-neutral-400 hover:text-neutral-700 dark:border-neutral-600 dark:text-neutral-400 dark:hover:border-neutral-500"
        >
          <Plus size={10} />
          Tag
        </button>
      </div>

      {open && (
        <div className="absolute left-0 top-full z-20 mt-2 w-56 rounded-lg border border-neutral-200 bg-white p-2 shadow-lg dark:border-neutral-700 dark:bg-neutral-800">
          {availableTags.length > 0 && (
            <div className="mb-2 max-h-32 space-y-1 overflow-y-auto">
              {availableTags.map((tag) => (
                <button
                  key={tag.id}
                  onClick={() => handleAdd(tag.id)}
                  className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs transition-colors hover:bg-neutral-100 dark:hover:bg-neutral-700"
                >
                  <TagIcon size={12} style={{ color: tag.color }} />
                  <span className="text-neutral-700 dark:text-neutral-300">
                    {tag.name}
                  </span>
                </button>
              ))}
            </div>
          )}
          <div className="flex items-center gap-1.5 border-t border-neutral-100 pt-2 dark:border-neutral-700">
            <input
              type="text"
              value={newTagName}
              onChange={(e) => setNewTagName(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleCreate()}
              placeholder="New tag..."
              className="flex-1 rounded border border-neutral-200 bg-transparent px-2 py-1 text-xs text-neutral-900 placeholder-neutral-400 focus:border-blue-500 focus:outline-none dark:border-neutral-600 dark:text-white"
            />
            <button
              onClick={handleCreate}
              disabled={creating || !newTagName.trim()}
              className="rounded bg-blue-600 px-2 py-1 text-xs font-medium text-white disabled:opacity-50"
            >
              Add
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
