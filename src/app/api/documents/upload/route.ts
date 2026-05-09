import { NextRequest, NextResponse } from "next/server";
import { getSupabase } from "@/lib/supabase";
import { v4 as uuidv4 } from "uuid";
import { writeFile, mkdir } from "fs/promises";
import { join } from "path";
import { config } from "@/lib/config";
import type { Document, UploadResponse } from "@/lib/types";

export async function POST(request: NextRequest) {
  try {
    const formData = await request.formData();
    const file = formData.get("file") as File | null;

    if (!file) {
      return NextResponse.json({ error: "No file provided" }, { status: 400 });
    }

    if (!file.name.toLowerCase().endsWith(".pdf")) {
      return NextResponse.json(
        { error: "Only PDF files are supported" },
        { status: 400 }
      );
    }

    if (file.size > config.upload.maxSizeMB * 1024 * 1024) {
      return NextResponse.json(
        { error: `File size exceeds ${config.upload.maxSizeMB}MB limit` },
        { status: 400 }
      );
    }

    const documentId = uuidv4();
    const uploadDir = join(/* turbopackIgnore: true */ process.cwd(), config.upload.dir);
    await mkdir(uploadDir, { recursive: true });

    const filePath = join(uploadDir, `${documentId}.pdf`);
    const buffer = Buffer.from(await file.arrayBuffer());
    await writeFile(filePath, buffer);

    const supabase = getSupabase();
    const { data, error } = await supabase
      .from("documents")
      .insert({
        id: documentId,
        name: file.name,
        file_path: filePath,
        file_size: file.size,
        status: "pending",
        page_count: 0,
      })
      .select()
      .single();

    if (error) {
      return NextResponse.json(
        { error: `Database error: ${error.message}` },
        { status: 500 }
      );
    }

    const response: UploadResponse = {
      document: data as Document,
      message: "Document uploaded successfully. Processing will begin shortly.",
    };

    return NextResponse.json(response, { status: 201 });
  } catch (err) {
    console.error("Upload error:", err);
    return NextResponse.json(
      { error: "Failed to upload document" },
      { status: 500 }
    );
  }
}
