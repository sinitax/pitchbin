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

test:
    go test -count=1 ./...

test-cli: build
    #!/usr/bin/env bash
    set -e
    rm -f pitchbin-test.db*
    ./pitchbin -addr :18956 -base-url http://localhost:18956 -pow-bits 8 -annotation-pow-bits 8 -rate-limit 100 -db pitchbin-test.db &
    PID=$!
    trap "kill $PID 2>/dev/null; rm -f pitchbin-test.db*" EXIT
    sleep 0.5
    node cli/test.mjs

test-all: test test-cli

check: fmt vet test

act:
    act push -W .github/workflows/test.yml

dev: build
    #!/usr/bin/env bash
    set -e
    ./pitchbin -addr :8080 -base-url http://localhost:8080 &
    PID=$!
    trap "kill $PID 2>/dev/null; exit" EXIT INT TERM
    while inotifywait -q -r -e modify,create,delete --include '\.(go|html|css|js)$' . ; do
        kill $PID 2>/dev/null
        wait $PID 2>/dev/null || true
        go build -o pitchbin . && {
            ./pitchbin -addr :8080 -base-url http://localhost:8080 &
            PID=$!
        }
    done

deploy HOST:
    ssh {{HOST}} "cd pitchbin && ln -sf compose.traefik.yaml docker/compose.override.yaml && docker compose -f docker/compose.yaml up -d --build"
