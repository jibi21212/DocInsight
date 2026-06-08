import { Search, FileText, Sparkles, ListOrdered, Crosshair, Wrench, Check } from "lucide-react";

type Props = {
  tools: { name: string; label: string; done: boolean }[];
};

function iconFor(name: string) {
  switch (name) {
    case "search_documents":
      return <Search size={12} />;
    case "get_document":
      return <FileText size={12} />;
    case "summarize_document":
      return <Sparkles size={12} />;
    case "list_documents":
      return <ListOrdered size={12} />;
    case "get_chunk_context":
      return <Crosshair size={12} />;
    default:
      return <Wrench size={12} />;
  }
}

export function ToolStatus({ tools }: Props) {
  if (tools.length === 0) return null;
  return (
    <div className="space-y-1">
      {tools.map((t, i) => (
        <div
          key={i}
          className={`flex items-center gap-2 rounded-md px-2 py-1 text-xs ${
            t.done
              ? "bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300"
              : "bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300"
          }`}
        >
          <span className="inline-flex h-4 w-4 items-center justify-center">
            {t.done ? <Check size={12} /> : iconFor(t.name)}
          </span>
          <span className="truncate">{t.label}</span>
          {!t.done && (
            <span
              aria-hidden
              className="ml-auto h-1.5 w-1.5 animate-pulse rounded-full bg-blue-500"
            />
          )}
        </div>
      ))}
    </div>
  );
}
