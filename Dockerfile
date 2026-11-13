FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/server ./cmd/server

FROM alpine:3.19
RUN adduser -D -u 10001 dartotp
USER dartotp
COPY --from=build /bin/server /usr/local/bin/server
EXPOSE 8080
ENTRYPOINT ["server"]
