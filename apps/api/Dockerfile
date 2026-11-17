# Build the Go API as a static binary, then run it in a minimal image.
#
# The build stage needs cgo disabled, so the binary has no dependency on a
# C runtime and can run on a distroless base.

FROM golang:1.27.4-alpine AS build

WORKDIR /src

# Download dependencies first so they are cached independently of source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 produces a static binary. -trimpath removes local filesystem
# paths, and the build flags come from the Go project rather than the image.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

# The application needs a writable directory only if it writes files; it does
# not, so the filesystem stays read-only in production.
COPY --from=build /out/server /server

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/server"]
