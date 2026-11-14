# SPDX-FileCopyrightText: The pacboard Authors
#
# SPDX-License-Identifier: AGPL-3.0-or-later

# ---- Frontend build stage ---------------------------------------------------
# Builds the Vue 3 SPA so web/dist/ is populated before Go's //go:embed runs.
FROM --platform=$BUILDPLATFORM docker.io/oven/bun:1-alpine@sha256:4cf82f5686b196504fe119db3fcd9185b191f82ece82c40680e597228ec6070a AS web-builder

WORKDIR /web

COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile

COPY web/ ./
RUN bun run build

# ---- Backend build stage ----------------------------------------------------
FROM --platform=$BUILDPLATFORM docker.io/golang:1.27-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Replace web/dist with the freshly built assets so //go:embed all:dist picks up the real SPA.
COPY --from=web-builder /web/dist ./web/dist

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG TARGETOS=linux
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /pacboard ./cmd/pacboard

# ---- Runtime stage ----------------------------------------------------------
# Shell-less, non-root (uid 65532). pacboard needs no writable filesystem.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.title="pacboard" \
      org.opencontainers.image.description="Dashboard for Tekton Pipelines-as-Code runs" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.source="https://github.com/M0Rf30/pacboard"

COPY --from=builder /pacboard /pacboard

EXPOSE 8080
# Numeric, so the kubelet can verify runAsNonRoot ("nonroot" by name is rejected).
USER 65532:65532
ENTRYPOINT ["/pacboard"]
