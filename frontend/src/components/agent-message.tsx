import { useState } from "react";
import { Link } from "react-router-dom";
import { User, Sparkles, FileText } from "lucide-react";
import type { AgentMessage, Citation } from "@/lib/types";

interface AgentMessageProps {
  message: AgentMessage;
}

// Replace <cite chunk="UUID"/> markers with numbered superscripts.
// Returns the rendered text fragments and an ordered list of citation chunk IDs.
function renderWithCitations(
  content: string,
  citations: Citation[] | undefined,
): { nodes: React.ReactNode[]; ordered: Citation[] } {
  if (!citations || citations.length === 0) {
    return { nodes: [content], ordered: [] };
  }
  const byChunk = new Map(citations.map((c) => [c.chunk_id, c]));
  const ordered: Citation[] = [];
  const indexFor = new Map<string, number>();

  const re = /<cite chunk="([0-9a-fA-F-]+)"\s*\/?>/g;
  const nodes: React.ReactNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  let key = 0;
  while ((match = re.exec(content)) !== null) {
    const before = content.slice(lastIndex, match.index);
    if (before) nodes.push(<span key={`t-${key++}`}>{before}</span>);
    const chunkID = match[1];
    const cit = byChunk.get(chunkID);
    if (cit) {
      let idx = indexFor.get(chunkID);
      if (idx === undefined) {
        ordered.push(cit);
        idx = ordered.length;
        indexFor.set(chunkID, idx);
      }
      nodes.push(
        <sup
          key={`c-${key++}`}
          className="ml-0.5 inline-flex h-4 min-w-4 items-center justify-center rounded bg-blue-100 px-1 text-[10px] font-semibold text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
          title={cit.document_name}
        >
          {idx}
        </sup>,
      );
    }
    lastIndex = match.index + match[0].length;
  }
  const tail = content.slice(lastIndex);
  if (tail) nodes.push(<span key={`t-${key++}`}>{tail}</span>);
  return { nodes, ordered };
}

export function AgentMessageView({ message }: AgentMessageProps) {
  const [expandedCit, setExpandedCit] = useState<string | null>(null);

  if (message.role === "user") {
    return (
      <div className="flex gap-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-neutral-200 text-neutral-700 dark:bg-neutral-700 dark:text-neutral-200">
          <User size={14} />
        </div>
        <div className="rounded-2xl rounded-tl-sm bg-neutral-100 px-4 py-2.5 text-sm text-neutral-900 dark:bg-neutral-800 dark:text-white">
          {message.content}
        </div>
      </div>
    );
  }

  if (message.role === "tool") {
    return null; // tool messages are internal
  }

  const { nodes, ordered } = renderWithCitations(
    message.content,
    message.citations,
  );

  return (
    <div className="flex gap-3">
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
        <Sparkles size={14} />
      </div>
      <div className="min-w-0 flex-1">
        <div className="whitespace-pre-wrap rounded-2xl rounded-tl-sm bg-white px-4 py-2.5 text-sm leading-relaxed text-neutral-900 ring-1 ring-neutral-200 dark:bg-neutral-900 dark:text-neutral-100 dark:ring-neutral-700">
          {nodes}
        </div>

        {ordered.length > 0 && (
          <div className="mt-2 space-y-1.5">
            <div className="text-[11px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
              Sources
            </div>
            {ordered.map((c, i) => (
              <div
                key={c.chunk_id}
                className="rounded-lg border border-neutral-200 bg-neutral-50 p-2 text-xs dark:border-neutral-700 dark:bg-neutral-900/40"
              >
                <button
                  type="button"
                  onClick={() =>
                    setExpandedCit((prev) =>
                      prev === c.chunk_id ? null : c.chunk_id,
                    )
                  }
                  className="flex w-full items-start gap-2 text-left"
                >
                  <span className="inline-flex h-4 min-w-4 shrink-0 items-center justify-center rounded bg-blue-100 px-1 text-[10px] font-semibold text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
                    {i + 1}
                  </span>
                  <span className="flex-1">
                    <Link
                      to={`/documents/${c.document_id}`}
                      className="flex items-center gap-1 font-medium text-blue-600 hover:underline dark:text-blue-400"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <FileText size={11} />
                      {c.document_name}
                    </Link>
                    {c.page_number > 0 && (
                      <span className="ml-1 text-neutral-500 dark:text-neutral-400">
                        · page {c.page_number}
                      </span>
                    )}
                  </span>
                </button>
                {expandedCit === c.chunk_id && (
                  <p className="mt-1.5 border-l-2 border-blue-300 pl-2 text-neutral-700 dark:text-neutral-300">
                    {c.snippet}
                  </p>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
