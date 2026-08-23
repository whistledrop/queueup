# The QueueUp relay, as one small container. The web app deploys to Vercel
# separately; this is only the Go relay.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /relay ./cmd/relay

FROM alpine:3.20
RUN adduser -D -H queueup
COPY --from=build /relay /usr/local/bin/relay
USER queueup
# The database lives on a mounted volume so it survives redeploys.
ENV QUEUEUP_DB=/data/queueup.db
ENV QUEUEUP_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["relay"]
CMD ["serve"]
