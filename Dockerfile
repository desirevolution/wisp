FROM --platform=$BUILDPLATFORM node:24-bookworm-slim AS frontend
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY frontend ./frontend
COPY internal/model/emojis.json ./internal/model/emojis.json
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=frontend /src/public ./internal/webui/public
ARG TARGETOS=linux
ARG TARGETARCH
ARG TARGETVARIANT
RUN set -eux; \
    arch="${TARGETARCH:-$(go env GOARCH)}"; \
    case "$arch/$TARGETVARIANT" in \
      arm/v7) goarm=7 ;; \
      arm/v6) goarm=6 ;; \
      *) goarm="" ;; \
    esac; \
    CGO_ENABLED=0 GOOS="${TARGETOS:-linux}" GOARCH="$arch" GOARM="$goarm" \
      go build -trimpath -ldflags="-s -w" -o /out/wisp ./cmd/wisp

# Export only the executable, never the complete builder filesystem.
# This stage stays native even when TARGETOS is Windows.
FROM --platform=$BUILDPLATFORM scratch AS binaries
COPY --from=backend /out/wisp /wisp

FROM gcr.io/distroless/static-debian13
COPY --from=backend /out/wisp /wisp
USER 1000:1000
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/wisp"]
