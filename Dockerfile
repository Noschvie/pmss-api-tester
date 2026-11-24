# syntax=docker/dockerfile:1

FROM golang:1.27.1-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN mkdir -p /out \
    && go generate ./... \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pmss-api-tester ./cmd/pmss-api-tester

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/pmss-api-tester /pmss-api-tester
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/pmss-api-tester"]
