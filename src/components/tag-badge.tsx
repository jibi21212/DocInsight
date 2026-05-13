"use client";

import { X } from "lucide-react";
import type { Tag } from "@/lib/types";

interface TagBadgeProps {
  tag: Tag;
  onRemove?: (tagId: string) => void;
  size?: "sm" | "md";
}

export function TagBadge({ tag, onRemove, size = "sm" }: TagBadgeProps) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full font-medium ${
        size === "sm" ? "px-2 py-0.5 text-xs" : "px-2.5 py-1 text-xs"
      }`}
      style={{
        backgroundColor: `${tag.color}20`,
        color: tag.color,
        border: `1px solid ${tag.color}40`,
      }}
    >
      {tag.name}
      {onRemove && (
        <button
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onRemove(tag.id);
          }}
          className="ml-0.5 rounded-full p-0.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
        >
          <X size={10} />
        </button>
      )}
    </span>
  );
}
