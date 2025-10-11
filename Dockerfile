########################
# Build stage
########################
FROM golang:1.24-alpine AS build
WORKDIR /src

COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Код прокси
COPY api ./api

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./api/cmd/llm-proxy

# -------- runtime stage --------
FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/server /app/server

ENV PORT=8000
EXPOSE 8000

USER 65532:65532
ENTRYPOINT ["/app/server"]