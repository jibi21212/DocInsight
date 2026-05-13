import Link from "next/link";
import { FileText, Globe, Hash } from "lucide-react";
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

function MatchTypeBadge({ type }: { type?: string }) {
  if (!type) return null;
  const styles: Record<string, string> = {
    semantic: "bg-violet-100 text-violet-700 dark:bg-violet-900/40 dark:text-violet-400",
    keyword: "bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-400",
    hybrid: "bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-400",
  };
  return (
    <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${styles[type] ?? styles.semantic}`}>
      {type.charAt(0).toUpperCase() + type.slice(1)}
    </span>
  );
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
            {result.source_type === "web" ? <Globe size={14} /> : <FileText size={14} />}
            {result.document_name}
          </Link>
          {result.source_type === "web" && (
            <span className="rounded bg-emerald-100 px-1.5 py-0.5 text-xs text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-400">
              Web
            </span>
          )}
          <MatchTypeBadge type={result.match_type} />
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
