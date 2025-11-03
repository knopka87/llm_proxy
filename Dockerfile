########################
# Build stage
########################
FROM golang:1.24-alpine AS build
WORKDIR /src

# Сборка без сети: используем вендоренные зависимости
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GOFLAGS="-mod=vendor"

# Если vendor уже в репо — отдельного go mod download не нужно
COPY go.mod go.sum ./
COPY vendor/ ./vendor/

# Копируем остальной код
COPY . .

# Сборка
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/server ./api/cmd/llm-proxy

RUN mkdir -p /out/prompts/v1/ocr/gpt/prompt && cp -r api/internal/v1/ocr/gpt/prompt/. /out/prompts/v1/ocr/gpt/prompt/
RUN mkdir -p /out/prompts/v2/ocr/gpt/prompt && cp -r api/internal/v2/ocr/gpt/prompt/. /out/prompts/v2/ocr/gpt/prompt/

########################
# Runtime stage
########################
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build --chown=nonroot:nonroot /out/prompts/ /app/prompts/
ENV PROMPT_DIR=/app/prompts
ENV PORT=8000
EXPOSE 8000
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]