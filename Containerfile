# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:26.5.1-alpine AS web-build
WORKDIR /src
COPY package.json package-lock.json ./
COPY web/package.json web/package.json
RUN npm ci
COPY web web
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
COPY web/embed.go web/embed.go
COPY --from=web-build /src/web/dist web/dist
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags "-s -w -X portinaia/internal/version.Version=${VERSION} -X portinaia/internal/version.Commit=${COMMIT} -X portinaia/internal/version.Date=${BUILD_DATE}" \
    -o /portinaia ./cmd/portinaia

FROM scratch
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=go-build /portinaia /portinaia
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/portinaia"]
CMD ["server"]
