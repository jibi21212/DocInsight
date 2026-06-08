import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  ArrowLeft,
  FileText,
  Calendar,
  HardDrive,
  Layers,
  Hash,
} from "lucide-react";
import { StatusBadge } from "@/components/status-badge";
import { TagManager } from "@/components/tag-manager";
import { fetchDocumentDetail } from "@/lib/api";
import type { Document, Tag, ChunkMetadata } from "@/lib/types";

interface ChunkData {
  id: string;
  content: string;
  page_number: number;
  chunk_index: number;
  metadata: ChunkMetadata;
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    weekday: "long",
    month: "long",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export default function DocumentDetailPage() {
  const { id } = useParams() as { id: string };
  const [document, setDocument] = useState<Document | null>(null);
  const [chunks, setChunks] = useState<ChunkData[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedChunks, setExpandedChunks] = useState<Set<string>>(new Set());
  const [tags, setTags] = useState<Tag[]>([]);

  useEffect(() => {
    async function load() {
      try {
        const res = await fetchDocumentDetail(id);
        setDocument(res.document);
        setChunks(res.chunks as unknown as ChunkData[]);
      } catch {
        setError("Failed to load document details");
      } finally {
        setLoading(false);
      }
    }
    load();
  }, [id]);

  const toggleChunk = (chunkId: string) => {
    setExpandedChunks((prev) => {
      const next = new Set(prev);
      if (next.has(chunkId)) {
        next.delete(chunkId);
      } else {
        next.add(chunkId);
      }
      return next;
    });
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-24">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-blue-600 border-t-transparent" />
      </div>
    );
  }

  if (error || !document) {
    return (
      <div className="py-24 text-center">
        <p className="text-red-600 dark:text-red-400">{error ?? "Document not found"}</p>
        <Link
          to="/"
          className="mt-4 inline-flex items-center gap-2 text-sm text-blue-600 hover:underline dark:text-blue-400"
        >
          <ArrowLeft size={14} />
          Back to Dashboard
        </Link>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <Link
        to="/"
        className="inline-flex items-center gap-2 text-sm text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-white"
      >
        <ArrowLeft size={14} />
        Back to Dashboard
      </Link>

      <div className="rounded-xl border border-neutral-200 bg-white p-6 dark:border-neutral-800 dark:bg-neutral-900">
        <div className="flex items-start gap-4">
          <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400">
            <FileText size={24} />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-start justify-between gap-3">
              <h1 className="text-xl font-bold text-neutral-900 dark:text-white">
                {document.name}
              </h1>
              <StatusBadge status={document.status} />
            </div>

            <div className="mt-3 flex flex-wrap gap-4 text-sm text-neutral-500 dark:text-neutral-400">
              <span className="flex items-center gap-1.5">
                <Calendar size={14} />
                {formatDate(document.created_at)}
              </span>
              <span className="flex items-center gap-1.5">
                <HardDrive size={14} />
                {formatFileSize(document.file_size)}
              </span>
              {document.page_count > 0 && (
                <span className="flex items-center gap-1.5">
                  <Layers size={14} />
                  {document.page_count} pages
                </span>
              )}
              <span className="flex items-center gap-1.5">
                <Hash size={14} />
                {chunks.length} chunks
              </span>
            </div>

            <div className="mt-3">
              <TagManager
                documentId={id}
                documentTags={tags}
                onTagsChange={setTags}
              />
            </div>
          </div>
        </div>

        {document.error_message && (
          <div className="mt-4 rounded-lg bg-red-50 px-4 py-3 text-sm text-red-600 dark:bg-red-900/20 dark:text-red-400">
            <strong>Error:</strong> {document.error_message}
          </div>
        )}
      </div>

      {chunks.length > 0 && (
        <section>
          <h2 className="mb-4 text-lg font-semibold text-neutral-900 dark:text-white">
            Document Chunks
          </h2>
          <div className="space-y-3">
            {chunks.map((chunk) => {
              const isExpanded = expandedChunks.has(chunk.id);
              const isLong = chunk.content.length > 300;
              const displayText =
                isLong && !isExpanded
                  ? chunk.content.slice(0, 300) + "..."
                  : chunk.content;

              return (
                <div
                  key={chunk.id}
                  className="rounded-xl border border-neutral-200 bg-white p-4 dark:border-neutral-800 dark:bg-neutral-900"
                >
                  <div className="mb-2 flex items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400">
                    <span className="rounded bg-neutral-100 px-2 py-0.5 font-medium dark:bg-neutral-800">
                      Chunk #{chunk.chunk_index}
                    </span>
                    <span>Page {chunk.page_number}</span>
                    <span>{chunk.metadata?.word_count ?? 0} words</span>
                  </div>
                  <p className="whitespace-pre-wrap text-sm leading-relaxed text-neutral-700 dark:text-neutral-300">
                    {displayText}
                  </p>
                  {isLong && (
                    <button
                      onClick={() => toggleChunk(chunk.id)}
                      className="mt-2 text-xs font-medium text-blue-600 hover:underline dark:text-blue-400"
                    >
                      {isExpanded ? "Show less" : "Show more"}
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        </section>
      )}
    </div>
  );
}
