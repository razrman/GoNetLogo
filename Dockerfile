# syntax=docker/dockerfile:1

# GoNetLogo web server.
#
# Stage 1 builds the React UI with Node 22 (Vite needs 22.12 or newer).
# Stage 2 compiles the Go server with CGO_ENABLED=0 and -tags web, which
# embeds that UI. The server package has no version variable, so the
# linker flags are -s -w only; there is nothing to set with -X.
# Stage 3 is a distroless nonroot image: the static binary plus models/
# at /models. The Fyne desktop app (cmd/gonetlogo, internal/gui) is not
# copied and is not compiled.
#
# linux/amd64 and linux/arm64: the Node and Go stages run on the builder
# (BUILDPLATFORM) and the Go compile sets GOOS/GOARCH to the target.
# The runtime image is the target platform. Official node, golang, and
# distroless images publish both architectures.
#
# /tmp must be writable. newSession calls os.MkdirTemp("", "gonetlogo-session-")
# for each session's scratch directory (uploaded model text). NetLogo
# file-open creates a relative path in the working directory, and
# os.CreateTemp (the sr extension) also uses TMPDIR. This image sets
# WORKDIR and TMPDIR to /tmp. The distroless nonroot image ships /tmp
# mode 1777. On ECS, mount a tmpfs at /tmp or rely on Fargate ephemeral
# storage. A read-only root with no writable /tmp leaves sessions without
# a scratch directory, and file-open cannot create a file.
#
#   docker build -t gonetlogo .
#   docker buildx build --platform linux/amd64,linux/arm64 -t gonetlogo .

FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build \
 && test -f dist/index.html

FROM --platform=$BUILDPLATFORM golang:1.22.2-bookworm AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY backend ./backend
COPY pkg ./pkg
COPY internal/logcfg ./internal/logcfg
# *.nlogo is what //go:embed in models pulls in. library/ is not embedded;
# the runtime stage copies it to /models.
COPY models/*.go models/*.nlogo ./models/
COPY --from=frontend /src/frontend/dist ./backend/cmd/server/webdist

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -tags web -trimpath -ldflags '-s -w' \
      -o /out/gonetlogo-server ./backend/cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/gonetlogo-server /gonetlogo-server
COPY models /models
ENV MODELS_DIR=/models \
    TMPDIR=/tmp
WORKDIR /tmp
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gonetlogo-server", "-addr", "0.0.0.0:8080", "-log-format", "json"]
