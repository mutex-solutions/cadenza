# Cadenza server image. Build from the repository root:
#   docker build -t cadenza .
# The web app is built in the first stage and embedded in the binary.

FROM node:24.11.1-alpine AS web
WORKDIR /src/web
RUN corepack enable
COPY web/ ./
RUN pnpm install --frozen-lockfile && pnpm --filter @cadenza/web build

FROM golang:1.26.2-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/apps/web/dist/ internal/server/webapp/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -tags nodynamic -ldflags "-s -w -X main.Version=${VERSION}" -o /cadenza-server ./cmd/cadenza-server
# An empty data folder owned by the distroless "nonroot" user (65532), so a
# fresh volume mounted at /data is writable.
RUN mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /cadenza-server /cadenza-server
COPY --from=server --chown=65532:65532 /data /data
ENV CADENZA_ADDR=:8080 CADENZA_DATA_DIR=/data
VOLUME /data
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/cadenza-server"]
