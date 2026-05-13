"use client";

import { useState } from "react";
import { Search } from "lucide-react";
import { SearchBar } from "@/components/search-bar";
import { SearchResultCard } from "@/components/search-result-card";
import { DocumentFilter } from "@/components/document-filter";
import { EmptyState } from "@/components/empty-state";
import { ExportToolbar } from "@/components/export-toolbar";
import { searchDocuments } from "@/store/app-store";
import type { SearchResult, SearchMode } from "@/lib/types";

export default function SearchPage() {
  const [results, setResults] = useState<SearchResult[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [tookMs, setTookMs] = useState(0);
  const [searched, setSearched] = useState(false);
  const [selectedDocIds, setSelectedDocIds] = useState<string[]>([]);

  const handleSearch = async (
    searchQuery: string,
    topK: number,
    threshold: number,
    searchMode?: SearchMode
  ) => {
    setLoading(true);
    setQuery(searchQuery);
    setSearched(true);
    try {
      const res = await searchDocuments(
        searchQuery,
        topK,
        threshold,
        selectedDocIds.length > 0 ? selectedDocIds : undefined,
        searchMode
      );
      setResults(res.results);
      setTookMs(res.took_ms);
    } catch (err) {
      console.error("Search failed:", err);
      setResults([]);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-neutral-900 dark:text-white">
          Semantic Search
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Search across all your documents using natural language. Results are
          ranked by semantic similarity.
        </p>
      </div>

      <SearchBar onSearch={handleSearch} loading={loading} />

      <DocumentFilter
        selectedIds={selectedDocIds}
        onChange={setSelectedDocIds}
      />

      {searched && !loading && (
        <div className="flex items-center justify-between gap-4 text-sm text-neutral-500 dark:text-neutral-400">
          <div className="flex items-center gap-4">
            <span>
              {results.length} result{results.length !== 1 ? "s" : ""} for{" "}
              <span className="font-medium text-neutral-900 dark:text-white">
                &quot;{query}&quot;
              </span>
            </span>
            <span>({tookMs}ms)</span>
          </div>
          <ExportToolbar results={results} query={query} />
        </div>
      )}

      {searched && !loading && results.length === 0 && (
        <EmptyState
          icon={Search}
          title="No results found"
          description="Try a different query or adjust the similarity threshold in the filters."
        />
      )}

      {results.length > 0 && (
        <div className="space-y-4">
          {results.map((result, i) => (
            <SearchResultCard
              key={result.chunk_id}
              result={result}
              index={i}
              query={query}
            />
          ))}
        </div>
      )}
    </div>
  );
}
