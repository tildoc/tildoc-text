# tildoc-text: text extraction, talking to tildoc over REST.
FROM golang:1.27-trixie AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tildoc-text .

FROM debian:trixie-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates poppler-utils \
 && rm -rf /var/lib/apt/lists/* \
 && useradd -r -m tildoctext
COPY --from=build /out/tildoc-text /usr/local/bin/tildoc-text
USER tildoctext
CMD ["tildoc-text"]
