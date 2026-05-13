"""Embedding sidecar server for DocInsight.

Provides a simple HTTP API to generate embeddings using sentence-transformers.
Uses the same model (all-MiniLM-L6-v2) as the original @xenova/transformers setup.
"""

import os
from contextlib import asynccontextmanager

import uvicorn
from fastapi import FastAPI
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer

MODEL_NAME = os.getenv("EMBEDDING_MODEL", "all-MiniLM-L6-v2")
PORT = int(os.getenv("SIDECAR_PORT", "8000"))

model: SentenceTransformer | None = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global model
    print(f"Loading embedding model: {MODEL_NAME}")
    model = SentenceTransformer(MODEL_NAME)
    print(f"Model loaded. Embedding dimension: {model.get_sentence_embedding_dimension()}")
    yield
    print("Shutting down embedding sidecar")


app = FastAPI(title="DocInsight Embedding Sidecar", lifespan=lifespan)


class EmbedRequest(BaseModel):
    texts: list[str]


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]


@app.get("/health")
def health():
    return {"status": "ok", "model": MODEL_NAME}


@app.post("/embed", response_model=EmbedResponse)
def embed(req: EmbedRequest):
    if not req.texts:
        return EmbedResponse(embeddings=[])

    embeddings = model.encode(
        req.texts,
        normalize_embeddings=True,
        show_progress_bar=False,
    )

    return EmbedResponse(embeddings=embeddings.tolist())


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=PORT)
