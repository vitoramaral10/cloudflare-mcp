# The builder runs on the build machine's platform and cross-compiles, so multi-arch
# images build without emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /cloudflare-mcp ./cmd/cloudflare-mcp

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /cloudflare-mcp /cloudflare-mcp
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/cloudflare-mcp"]
