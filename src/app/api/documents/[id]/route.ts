import { NextRequest, NextResponse } from "next/server";
import { getSupabase } from "@/lib/supabase";
import { unlink } from "fs/promises";
import type { Document } from "@/lib/types";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  try {
    const { id } = await params;
    const supabase = getSupabase();

    const { data: doc, error: docError } = await supabase
      .from("documents")
      .select("*")
      .eq("id", id)
      .single();

    if (docError || !doc) {
      return NextResponse.json(
        { error: "Document not found" },
        { status: 404 }
      );
    }

    // Fetch chunks for this document
    const { data: chunks, error: chunksError } = await supabase
      .from("chunks")
      .select("id, content, page_number, chunk_index, metadata, created_at")
      .eq("document_id", id)
      .order("chunk_index", { ascending: true });

    if (chunksError) {
      return NextResponse.json(
        { error: `Failed to fetch chunks: ${chunksError.message}` },
        { status: 500 }
      );
    }

    return NextResponse.json({
      document: doc as Document,
      chunks: chunks ?? [],
      chunkCount: chunks?.length ?? 0,
    });
  } catch (err) {
    console.error("Get document error:", err);
    return NextResponse.json(
      { error: "Failed to fetch document" },
      { status: 500 }
    );
  }
}

export async function DELETE(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  try {
    const { id } = await params;
    const supabase = getSupabase();

    // Get document to find file path
    const { data: doc, error: docError } = await supabase
      .from("documents")
      .select("file_path")
      .eq("id", id)
      .single();

    if (docError || !doc) {
      return NextResponse.json(
        { error: "Document not found" },
        { status: 404 }
      );
    }

    // Delete from database (cascading delete removes chunks and embeddings)
    const { error: deleteError } = await supabase
      .from("documents")
      .delete()
      .eq("id", id);

    if (deleteError) {
      return NextResponse.json(
        { error: `Failed to delete: ${deleteError.message}` },
        { status: 500 }
      );
    }

    // Delete file from disk
    try {
      await unlink(doc.file_path);
    } catch {
      // File may already be deleted, ignore
    }

    return NextResponse.json({ message: "Document deleted successfully" });
  } catch (err) {
    console.error("Delete document error:", err);
    return NextResponse.json(
      { error: "Failed to delete document" },
      { status: 500 }
    );
  }
}
