FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

FROM alpine:3.22

RUN apk add --no-cache ca-certificates postgresql-client

WORKDIR /app
RUN mkdir -p /app/uploads /app/backups

COPY --from=build /out/api /app/api

EXPOSE 8080
CMD ["/app/api"]
