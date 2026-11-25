# syntax=docker/dockerfile:1

# The build stage runs on the build platform and cross-compiles, so a
# multi-arch build (linux/amd64, linux/arm64) needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS TARGETARCH
ARG VERSION=dev
RUN mkdir -p /out \
    && go generate ./... \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
       go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/pmss-api-tester ./cmd/pmss-api-tester

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/pmss-api-tester /pmss-api-tester
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/pmss-api-tester"]
