# syntax=docker/dockerfile:1.7
FROM golang:1.26-bookworm AS go-build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/jobdock-builder ./cmd/jobdock-builder

FROM moby/buildkit:v0.30.0 AS buildkit

FROM debian:bookworm-slim
ARG VERSION=dev
LABEL org.opencontainers.image.title="JobDock Builder" \
      org.opencontainers.image.description="Isolated BuildKit client for JobDock source builds" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/Alejandro-GZ/JobDock"
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
RUN useradd --system --uid 10002 --home /var/lib/jobdock-builder jobdock-builder && mkdir -p /var/lib/jobdock-builder && chown -R jobdock-builder:jobdock-builder /var/lib/jobdock-builder
COPY --from=go-build /out/jobdock-builder /usr/local/bin/jobdock-builder
COPY --from=buildkit /usr/bin/buildctl /usr/local/bin/buildctl
USER 10002:10002
ENV JOBDOCK_BUILDER_STATE_DIR=/var/lib/jobdock-builder JOBDOCK_BUILDER_WORKSPACE_DIR=/var/lib/jobdock-builder/workspaces
VOLUME ["/var/lib/jobdock-builder"]
ENTRYPOINT ["jobdock-builder"]
