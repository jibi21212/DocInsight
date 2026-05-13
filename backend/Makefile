.PHONY: build run test clean sidecar sidecar-install dev

# Go backend
build:
	go build -o bin/server ./cmd/server

run: build
	./bin/server

test:
	go test ./...

clean:
	rm -rf bin/

# Embedding sidecar
sidecar-install:
	cd embedding-sidecar && pip install -r requirements.txt

sidecar:
	cd embedding-sidecar && python main.py

# Development: run both backend and sidecar
# (run these in separate terminals)
dev-backend: build
	./bin/server

dev-sidecar: sidecar
