import { pipeline } from "@xenova/transformers";
import { config } from "./config";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type EmbeddingPipeline = any;

let embeddingPipeline: EmbeddingPipeline = null;
let pipelineLoading: Promise<EmbeddingPipeline> | null = null;

async function getEmbeddingPipeline(): Promise<EmbeddingPipeline> {
  if (embeddingPipeline) return embeddingPipeline;

  if (pipelineLoading) return pipelineLoading;

  pipelineLoading = pipeline("feature-extraction", config.embedding.model, {
    quantized: true,
  });

  embeddingPipeline = await pipelineLoading;
  pipelineLoading = null;
  return embeddingPipeline;
}

export async function generateEmbedding(text: string): Promise<number[]> {
  const extractor = await getEmbeddingPipeline();

  const output = await extractor(text, {
    pooling: "mean",
    normalize: true,
  });

  return Array.from(output.data as Float32Array);
}

export async function generateEmbeddings(
  texts: string[],
  batchSize: number = 32
): Promise<number[][]> {
  const allEmbeddings: number[][] = [];

  for (let i = 0; i < texts.length; i += batchSize) {
    const batch = texts.slice(i, i + batchSize);
    const batchResults = await Promise.all(
      batch.map((text) => generateEmbedding(text))
    );
    allEmbeddings.push(...batchResults);
  }

  return allEmbeddings;
}
