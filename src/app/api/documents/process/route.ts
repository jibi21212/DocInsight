import { NextRequest, NextResponse } from "next/server";
import { readFile } from "fs/promises";
import { getSupabase } from "@/lib/supabase";
import { parsePdf } from "@/lib/pdf-parser";
import { createChunks } from "@/lib/chunker";
import { generateEmbeddings } from "@/lib/embeddings";
import type { Document } from "@/lib/types";

export async function POST(request: NextRequest) {
  try {
    const { documentId } = (await request.json()) as { documentId: string };

    if (!documentId) {
      return NextResponse.json(
        { error: "documentId is required" },
        { status: 400 }
      );
    }

    const supabase = getSupabase();

    // Fetch document
    const { data: doc, error: docError } = await supabase
      .from("documents")
      .select("*")
      .eq("id", documentId)
      .single();

    if (docError || !doc) {
      return NextResponse.json(
        { error: "Document not found" },
        { status: 404 }
      );
    }

    const document = doc as Document;

    if (document.status === "processing") {
      return NextResponse.json(
        { error: "Document is already being processed" },
        { status: 409 }
      );
    }

    // Mark as processing
    await supabase
      .from("documents")
      .update({ status: "processing" })
      .eq("id", documentId);

    // Start async processing (don't await - return immediately)
    processDocument(document, supabase).catch((err) => {
      console.error(`Processing failed for ${documentId}:`, err);
    });

    return NextResponse.json({
      message: "Processing started",
      documentId,
    });
  } catch (err) {
    console.error("Process route error:", err);
    return NextResponse.json(
      { error: "Failed to start processing" },
      { status: 500 }
    );
  }
}

async function processDocument(
  document: Document,
  supabase: ReturnType<typeof getSupabase>
) {
  const documentId = document.id;

  try {
    // Step 1: Extract text from PDF
    console.log(`[${documentId}] Extracting text from PDF...`);
    const fileBuffer = await readFile(document.file_path);
    const parsed = await parsePdf(fileBuffer);

    await supabase
      .from("documents")
      .update({ page_count: parsed.pageCount })
      .eq("id", documentId);

    // Step 2: Create chunks
    console.log(`[${documentId}] Creating chunks... (${parsed.pages.length} pages)`);
    const chunks = createChunks(parsed.pages);
    console.log(`[${documentId}] Created ${chunks.length} chunks`);

    if (chunks.length === 0) {
      await supabase
        .from("documents")
        .update({
          status: "failed",
          error_message: "No text content could be extracted from this PDF",
        })
        .eq("id", documentId);
      return;
    }

    // Step 3: Insert chunks into database
    console.log(`[${documentId}] Inserting chunks into database...`);
    const chunkRecords = chunks.map((chunk) => ({
      document_id: documentId,
      content: chunk.content,
      page_number: chunk.pageNumber,
      chunk_index: chunk.chunkIndex,
      metadata: chunk.metadata,
    }));

    const { data: insertedChunks, error: chunkError } = await supabase
      .from("chunks")
      .insert(chunkRecords)
      .select("id, content");

    if (chunkError) {
      throw new Error(`Failed to insert chunks: ${chunkError.message}`);
    }

    // Step 4: Generate embeddings
    console.log(`[${documentId}] Generating embeddings for ${insertedChunks.length} chunks...`);
    const texts = insertedChunks.map(
      (c: { id: string; content: string }) => c.content
    );
    const embeddings = await generateEmbeddings(texts);

    // Step 5: Store embeddings
    console.log(`[${documentId}] Storing embeddings...`);
    const embeddingRecords = insertedChunks.map(
      (chunk: { id: string; content: string }, index: number) => ({
        chunk_id: chunk.id,
        embedding: JSON.stringify(embeddings[index]),
      })
    );

    // Insert in batches of 50 to avoid payload limits
    for (let i = 0; i < embeddingRecords.length; i += 50) {
      const batch = embeddingRecords.slice(i, i + 50);
      const { error: embError } = await supabase
        .from("embeddings")
        .insert(batch);

      if (embError) {
        throw new Error(`Failed to insert embeddings batch: ${embError.message}`);
      }
    }

    // Mark as completed
    await supabase
      .from("documents")
      .update({ status: "completed" })
      .eq("id", documentId);

    console.log(`[${documentId}] Processing complete!`);
  } catch (err) {
    console.error(`[${documentId}] Processing error:`, err);
    const message = err instanceof Error ? err.message : "Unknown processing error";
    await supabase
      .from("documents")
      .update({ status: "failed", error_message: message })
      .eq("id", documentId);
  }
}
