default:
    @just --list

build:
    go build -o pitchbin .

run: build
    ./pitchbin -addr :8080 -base-url http://localhost:8080

fmt:
    gofmt -w .

vet:
    go vet ./...

tidy:
    go mod tidy

check: fmt vet
    go build ./...

deploy HOST:
    GOOS=linux GOARCH=amd64 go build -o pitchbin .
    scp pitchbin {{HOST}}:/usr/local/bin/pitchbin
    ssh {{HOST}} systemctl restart pitchbin
