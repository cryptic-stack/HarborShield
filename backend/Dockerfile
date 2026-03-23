FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -o /out/worker ./cmd/worker
RUN CGO_ENABLED=0 go build -o /out/blobnode ./cmd/blobnode

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget procps
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/worker /app/worker
COPY --from=build /out/blobnode /app/blobnode
COPY --from=build /src/migrations /app/migrations
EXPOSE 8080
CMD ["./api"]
