# -------- build stage --------
FROM golang:1.24-alpine AS build
WORKDIR /src

# модульные файлы из корня
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# исходники
COPY api ./api

# сборка бинарника из api/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./api

# -------- runtime stage --------
FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/server /app/server

ENV PORT=8080
EXPOSE 8080

USER 65532:65532
ENTRYPOINT ["/app/server"]
