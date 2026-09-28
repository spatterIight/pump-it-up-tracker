# The builder image can be swapped for a mirror of the same image.
ARG GO_IMAGE=docker.io/library/golang:1.27-alpine

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/pump-it-up-tracker ./cmd/pump-it-up-tracker

# Static binary, CA certificates for downloading art, no shell, non-root.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/pump-it-up-tracker /usr/local/bin/pump-it-up-tracker

ENV PIU_TRACKER_DATA_FILE=/config/tracker.json \
    PIU_TRACKER_ART_CACHE_DIR=/data/art \
    PIU_TRACKER_ART_CUSTOM_DIR=/art \
    PIU_TRACKER_LISTEN_ADDRESS=:8080

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/pump-it-up-tracker", "healthcheck"]

ENTRYPOINT ["/usr/local/bin/pump-it-up-tracker"]
CMD ["serve"]
