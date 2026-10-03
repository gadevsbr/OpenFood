FROM golang:1.26.8-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w -buildid=' -o /openfood ./cmd/openfood
FROM debian:bookworm-slim
RUN groupadd --gid 10001 openfood && useradd --uid 10001 --gid 10001 --no-create-home openfood
COPY --from=build /openfood /usr/local/bin/openfood
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/openfood"]
