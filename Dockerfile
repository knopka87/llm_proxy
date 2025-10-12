# Build stage
########################
FROM golang:1.24-alpine AS build
WORKDIR /src

# Tools needed for go mod and HTTPS
RUN apk add --no-cache git ca-certificates && update-ca-certificates

# 1) Dependencies (better cache)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 2) Source code (копируем весь модуль llm-proxy)
COPY . .

# 3) Build
RUN CGO_ENABLED=0 go build -o out/server api/cmd/llm-proxy/*.go

########################
# Runtime stage
########################
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server

ENV PORT=8000
EXPOSE 8000
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]