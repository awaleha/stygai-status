FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
      -o /out/stygai-status ./cmd/stygai-status \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
      -o /out/notify ./cmd/notify

FROM debian:bookworm-slim

RUN sed -i 's/^Components: main$/Components: main contrib/' /etc/apt/sources.list.d/debian.sources \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates \
      zfsutils-linux \
      systemd \
 && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /run/systemd/system

COPY --from=build /out/stygai-status /usr/local/bin/stygai-status

RUN useradd --uid 65532 --no-create-home --shell /usr/sbin/nologin app
USER app

ENTRYPOINT ["/usr/local/bin/stygai-status"]