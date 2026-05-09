import { PDFParse } from "pdf-parse";

export interface ParsedPage {
  pageNumber: number;
  text: string;
}

export interface ParsedDocument {
  text: string;
  pages: ParsedPage[];
  pageCount: number;
  metadata: {
    title?: string;
    author?: string;
    subject?: string;
    creator?: string;
  };
}

export async function parsePdf(buffer: Buffer): Promise<ParsedDocument> {
  const parser = new PDFParse({ data: new Uint8Array(buffer) });

  const [textResult, infoResult] = await Promise.all([
    parser.getText(),
    parser.getInfo(),
  ]);

  const pages: ParsedPage[] = textResult.pages
    .map((page) => ({
      pageNumber: page.num,
      text: page.text.trim(),
    }))
    .filter((page) => page.text.length > 0);

  // If no pages extracted but we have full text, use it as single page
  if (pages.length === 0 && textResult.text.trim().length > 0) {
    pages.push({ pageNumber: 1, text: textResult.text.trim() });
  }

  const info = infoResult.info ?? {};

  await parser.destroy();

  return {
    text: textResult.text,
    pages,
    pageCount: infoResult.total ?? pages.length,
    metadata: {
      title: info.Title as string | undefined,
      author: info.Author as string | undefined,
      subject: info.Subject as string | undefined,
      creator: info.Creator as string | undefined,
    },
  };
}
