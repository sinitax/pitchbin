FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /pitchbin .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /pitchbin /usr/local/bin/pitchbin
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["pitchbin"]
CMD ["-addr", ":8080", "-db", "/data/pitchbin.db"]
