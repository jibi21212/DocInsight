import { config } from "./config";
import type { ParsedPage } from "./pdf-parser";
import type { ChunkMetadata } from "./types";

export interface TextChunk {
  content: string;
  pageNumber: number;
  chunkIndex: number;
  metadata: ChunkMetadata;
}

const SENTENCE_ENDINGS = /(?<=[.!?])\s+/;
const PARAGRAPH_BREAK = /\n\s*\n/;

function splitIntoSentences(text: string): string[] {
  return text
    .split(SENTENCE_ENDINGS)
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

function splitIntoParagraphs(text: string): string[] {
  return text
    .split(PARAGRAPH_BREAK)
    .map((p) => p.trim())
    .filter((p) => p.length > 0);
}

export function createChunks(
  pages: ParsedPage[],
  chunkSize: number = config.chunking.chunkSize,
  chunkOverlap: number = config.chunking.chunkOverlap
): TextChunk[] {
  const chunks: TextChunk[] = [];
  let chunkIndex = 0;

  // Build a flat list of paragraphs tagged with their page number
  const taggedParagraphs: { text: string; pageNumber: number }[] = [];
  for (const page of pages) {
    const paragraphs = splitIntoParagraphs(page.text);
    for (const para of paragraphs) {
      taggedParagraphs.push({ text: para, pageNumber: page.pageNumber });
    }
  }

  let currentChunkParts: string[] = [];
  let currentLength = 0;
  let currentStartPage = taggedParagraphs[0]?.pageNumber ?? 1;
  let currentEndPage = currentStartPage;

  for (const { text, pageNumber } of taggedParagraphs) {
    // If a single paragraph exceeds chunk size, split it by sentences
    if (text.length > chunkSize) {
      // Flush current buffer first
      if (currentChunkParts.length > 0) {
        const content = currentChunkParts.join("\n\n");
        chunks.push(
          buildChunk(content, currentStartPage, currentEndPage, chunkIndex++)
        );
        // Keep overlap
        const overlapParts = getOverlapParts(currentChunkParts, chunkOverlap);
        currentChunkParts = overlapParts;
        currentLength = overlapParts.join("\n\n").length;
        currentStartPage = currentEndPage;
      }

      // Split large paragraph into sentence-level chunks
      const sentences = splitIntoSentences(text);
      let sentenceBuffer: string[] = [];
      let sentenceLength = 0;

      for (const sentence of sentences) {
        if (sentenceLength + sentence.length > chunkSize && sentenceBuffer.length > 0) {
          const content = sentenceBuffer.join(" ");
          chunks.push(buildChunk(content, pageNumber, pageNumber, chunkIndex++));
          // Keep overlap from sentences
          const overlapSentences = getOverlapParts(sentenceBuffer, chunkOverlap);
          sentenceBuffer = overlapSentences;
          sentenceLength = overlapSentences.join(" ").length;
        }
        sentenceBuffer.push(sentence);
        sentenceLength += sentence.length + 1;
      }

      // Remaining sentences become part of the next chunk
      if (sentenceBuffer.length > 0) {
        currentChunkParts = [sentenceBuffer.join(" ")];
        currentLength = currentChunkParts[0].length;
        currentStartPage = pageNumber;
        currentEndPage = pageNumber;
      }
      continue;
    }

    // Normal case: accumulate paragraphs
    if (currentLength + text.length + 2 > chunkSize && currentChunkParts.length > 0) {
      const content = currentChunkParts.join("\n\n");
      chunks.push(
        buildChunk(content, currentStartPage, currentEndPage, chunkIndex++)
      );
      // Keep overlap
      const overlapParts = getOverlapParts(currentChunkParts, chunkOverlap);
      currentChunkParts = overlapParts;
      currentLength = overlapParts.join("\n\n").length;
      currentStartPage = currentEndPage;
    }

    currentChunkParts.push(text);
    currentLength += text.length + 2;
    currentEndPage = pageNumber;
  }

  // Flush remaining
  if (currentChunkParts.length > 0) {
    const content = currentChunkParts.join("\n\n");
    chunks.push(
      buildChunk(content, currentStartPage, currentEndPage, chunkIndex++)
    );
  }

  return chunks;
}

function buildChunk(
  content: string,
  startPage: number,
  endPage: number,
  chunkIndex: number
): TextChunk {
  const words = content.split(/\s+/).filter((w) => w.length > 0);
  return {
    content,
    pageNumber: startPage,
    chunkIndex,
    metadata: {
      char_count: content.length,
      word_count: words.length,
      start_page: startPage,
      end_page: endPage,
    },
  };
}

function getOverlapParts(parts: string[], overlapSize: number): string[] {
  const result: string[] = [];
  let totalLength = 0;

  for (let i = parts.length - 1; i >= 0; i--) {
    if (totalLength >= overlapSize) break;
    result.unshift(parts[i]);
    totalLength += parts[i].length;
  }

  return result;
}
