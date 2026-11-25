FROM alpine:3.24 AS runtime
LABEL org.opencontainers.image.title="g10k" \
  org.opencontainers.image.description="A Go implementation of r10k for Puppet environment deployment" \
  org.opencontainers.image.source="https://github.com/voxpupuli/g10k" \
  org.opencontainers.image.url="https://github.com/voxpupuli/g10k" \
  org.opencontainers.image.vendor="Vox Pupuli" \
  org.opencontainers.image.licenses="Apache-2.0"
RUN apk add --no-cache bash ca-certificates git openssh-client-default && \
  addgroup -g 10001 g10k && \
  adduser -D -u 10001 -G g10k -h /home/g10k g10k && \
  mkdir -p /code /home/g10k/.cache/g10k/forge /home/g10k/.cache/g10k/modules && \
  chown g10k:g10k /code && \
  chown -R g10k:g10k /home/g10k
COPY LICENSE /usr/share/licenses/g10k/LICENSE
ENV HOME=/home/g10k
WORKDIR /code
USER 10001:10001
ENTRYPOINT ["/usr/bin/g10k"]
CMD ["-help"]

# GoReleaser supplies the binary for each target platform.
FROM runtime AS release
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/g10k /usr/bin/g10k

FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY pkg/ pkg/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.buildversion=${VERSION} -X main.buildtime=$(date -u '+%Y-%m-%d_%H:%M:%S')" \
  -o /out/g10k ./cmd/g10k

FROM runtime AS local
COPY --from=builder /out/g10k /usr/bin/g10k
