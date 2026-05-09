import { NextRequest, NextResponse } from "next/server";
import { getSupabase } from "@/lib/supabase";
import { generateEmbedding } from "@/lib/embeddings";
import { config } from "@/lib/config";
import type { SearchRequest, SearchResponse, SearchResult } from "@/lib/types";

export async function POST(request: NextRequest) {
  try {
    const body = (await request.json()) as SearchRequest;
    const {
      query,
      topK = config.search.topK,
      threshold = config.search.similarityThreshold,
      documentIds,
    } = body;

    if (!query || query.trim().length === 0) {
      return NextResponse.json(
        { error: "Query is required" },
        { status: 400 }
      );
    }

    const startTime = Date.now();

    // Generate embedding for the query
    const queryEmbedding = await generateEmbedding(query.trim());

    const supabase = getSupabase();

    // Call the match_embeddings function
    const { data, error } = await supabase.rpc("match_embeddings", {
      query_embedding: JSON.stringify(queryEmbedding),
      match_threshold: threshold,
      match_count: topK,
      filter_document_ids: documentIds ?? null,
    });

    if (error) {
      console.error("Search error:", error);
      return NextResponse.json(
        { error: `Search failed: ${error.message}` },
        { status: 500 }
      );
    }

    const results: SearchResult[] = (data ?? []).map(
      (row: Record<string, unknown>) => ({
        chunk_id: row.chunk_id as string,
        content: row.content as string,
        similarity: row.similarity as number,
        page_number: row.page_number as number,
        chunk_index: row.chunk_index as number,
        metadata: row.metadata as SearchResult["metadata"],
        document_id: row.document_id as string,
        document_name: row.document_name as string,
      })
    );

    const took_ms = Date.now() - startTime;

    const response: SearchResponse = {
      results,
      query: query.trim(),
      total: results.length,
      took_ms,
    };

    return NextResponse.json(response);
  } catch (err) {
    console.error("Search error:", err);
    return NextResponse.json(
      { error: "Search failed" },
      { status: 500 }
    );
  }
}
