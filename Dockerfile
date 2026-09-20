# syntax=docker/dockerfile:1

# Build on the native platform and cross-compile to the target platform, so
# multi-arch builds don't have to run the Go toolchain under emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG BUILDVERSION=dev
ARG BUILDTIME

WORKDIR /usr/src/g10k

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
  go build \
  -trimpath \
  -ldflags "-s -w -X main.buildversion=${BUILDVERSION} -X main.buildtime=${BUILDTIME}" \
  -o /out/g10k ./cmd/g10k

FROM alpine:3.23

# g10k shells out to git, which needs ssh for git+ssh:// module sources.
RUN apk add --no-cache ca-certificates git openssh-client bash && \
  adduser -D -u 1000 g10k

COPY --from=builder /out/g10k /usr/bin/g10k

LABEL org.opencontainers.image.title="g10k" \
  org.opencontainers.image.description="g10k is a r10k fork written in Go" \
  org.opencontainers.image.url="https://github.com/voxpupuli/g10k" \
  org.opencontainers.image.source="https://github.com/voxpupuli/g10k" \
  org.opencontainers.image.vendor="Vox Pupuli" \
  org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /code
RUN chown g10k:g10k /code
USER g10k

ENTRYPOINT [ "/usr/bin/g10k" ]
CMD ["-help"]
