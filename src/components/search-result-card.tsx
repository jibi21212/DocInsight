import Link from "next/link";
import { FileText, Hash } from "lucide-react";
import type { SearchResult } from "@/lib/types";

interface SearchResultCardProps {
  result: SearchResult;
  index: number;
  query: string;
}

function highlightQuery(text: string, query: string): React.ReactNode {
  if (!query.trim()) return text;

  const words = query
    .trim()
    .split(/\s+/)
    .filter((w) => w.length > 2);
  if (words.length === 0) return text;

  const pattern = new RegExp(`(${words.map(escapeRegex).join("|")})`, "gi");
  const parts = text.split(pattern);

  return parts.map((part, i) =>
    pattern.test(part) ? (
      <mark
        key={i}
        className="rounded bg-yellow-200 px-0.5 dark:bg-yellow-800/60 dark:text-yellow-200"
      >
        {part}
      </mark>
    ) : (
      part
    )
  );
}

function escapeRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function getSimilarityColor(score: number): string {
  if (score >= 0.8) return "text-green-600 dark:text-green-400";
  if (score >= 0.6) return "text-blue-600 dark:text-blue-400";
  if (score >= 0.4) return "text-yellow-600 dark:text-yellow-400";
  return "text-neutral-500 dark:text-neutral-400";
}

export function SearchResultCard({
  result,
  index,
  query,
}: SearchResultCardProps) {
  const similarityPercent = (result.similarity * 100).toFixed(1);

  return (
    <div className="rounded-xl border border-neutral-200 bg-white p-5 transition-all hover:shadow-md dark:border-neutral-800 dark:bg-neutral-900">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="flex h-6 w-6 items-center justify-center rounded-full bg-blue-100 text-xs font-bold text-blue-700 dark:bg-blue-900/40 dark:text-blue-400">
            {index + 1}
          </span>
          <Link
            href={`/documents/${result.document_id}`}
            className="flex items-center gap-1.5 text-sm font-medium text-blue-600 hover:underline dark:text-blue-400"
          >
            <FileText size={14} />
            {result.document_name}
          </Link>
        </div>
        <div className="flex items-center gap-3 text-xs">
          <span className="flex items-center gap-1 text-neutral-500 dark:text-neutral-400">
            <Hash size={12} />
            Page {result.page_number}
          </span>
          <span className={`font-semibold ${getSimilarityColor(result.similarity)}`}>
            {similarityPercent}% match
          </span>
        </div>
      </div>

      <div className="rounded-lg bg-neutral-50 p-4 text-sm leading-relaxed text-neutral-700 dark:bg-neutral-800/50 dark:text-neutral-300">
        {highlightQuery(result.content, query)}
      </div>

      {result.metadata && (
        <div className="mt-3 flex gap-4 text-xs text-neutral-400 dark:text-neutral-500">
          <span>{result.metadata.word_count} words</span>
          <span>Chunk #{result.chunk_index}</span>
          {result.metadata.start_page !== result.metadata.end_page && (
            <span>
              Pages {result.metadata.start_page}-{result.metadata.end_page}
            </span>
          )}
        </div>
      )}
    </div>
  );
}
