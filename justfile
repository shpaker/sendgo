# sendgo — задачи для разработки (just). Альтернатива Makefile.
#
# Все запуски — напрямую через бинарь, без docker-compose (бинарь самодостаточен:
# in-memory meta + FS blob по умолчанию).
#
# Примеры:
#   just                                                  # показать список
#   just dev                                              # быстрый go run с дефолтами
#   just run --redis-dsn=redis://localhost:6379           # доп. флаги форвардятся
#   just run --s3-bucket=sendgo --s3-endpoint=...         # S3-режим
#   just test                                             # юнит-тесты

# Default: показать список задач.
default:
    @just --list

# === сборка ===

# Сборка фронта (webpack → dist/).
build-fe:
    npm ci
    npm run build

# Скопировать dist/ и locales/ в server/static/ для //go:embed.
# Зависимость для build-be / run / dev / help.
prepare-static: build-fe
    rm -rf server/static/dist server/static/locales
    cp -r dist           server/static/dist
    cp -r public/locales server/static/locales

# Сборка бинаря sendgo.
build-be: prepare-static
    cd server && go build -trimpath -o ../bin/sendgo ./cmd/sendgo

# Полный билд (фронт + бекенд).
build: build-be

# === запуск бинаря ===

# Быстрый dev-цикл через `go run` (без отдельного билд-шага).
# In-memory meta + FS blob + cleanup, text-логи. Дополнительные флаги через `*args`:
#   just dev --log-level=debug --port=8080
dev *args: prepare-static
    cd server && go run ./cmd/sendgo \
        --cleanup-enabled \
        --file-dir=./tmp/blobs \
        --log-format=text \
        {{args}}

# Запустить уже собранный бинарь с дев-дефолтами.
# Прокидывает доп. флаги. Например:
#   just run --redis-dsn=redis://localhost:6379/0
#   just run --s3-bucket=sendgo --s3-endpoint=https://storage.yandexcloud.net
run *args: build-be
    ./bin/sendgo \
        --cleanup-enabled \
        --file-dir=./tmp/blobs \
        --log-format=text \
        {{args}}

# Прод-подобный запуск: json-логи, отдельный data-каталог.
run-prod *args: build-be
    ./bin/sendgo \
        --cleanup-enabled \
        --file-dir=./var/blobs \
        --log-format=json \
        {{args}}

# Показать --help (быстрый смоук CLI после правок флагов).
help: build-be
    ./bin/sendgo --help

# Показать версию.
version: build-be
    ./bin/sendgo --version

# === качество ===

test:
    cd server && go test ./... -race -count=1

# Coverage profile across the whole module + HTML heatmap. Open
# server/coverage.html for the per-line view.
test-cover:
    cd server && go test -race -count=1 -coverprofile=coverage.out ./...
    cd server && go tool cover -html=coverage.out -o coverage.html
    @echo "Wrote server/coverage.html"

# Strict gate: 100% line coverage of internal/usecase/... .
# Lists any function whose coverage is not exactly 100.0% and exits non-zero.
test-usecase:
    #!/usr/bin/env bash
    set -euo pipefail
    cd server
    go test -race -count=1 \
        -coverprofile=coverage-usecase.out \
        -coverpkg=./internal/usecase/... \
        ./internal/usecase/...
    uncovered=$(go tool cover -func=coverage-usecase.out \
        | grep -v '^total:' \
        | awk '$NF != "100.0%"' || true)
    if [ -n "$uncovered" ]; then
        echo "uncovered:"
        echo "$uncovered"
        exit 1
    fi
    echo "usecase coverage: 100.0%"

itest:
    cd server && go test -tags=integration ./test/integration/...

fmt:
    cd server && gofmt -w . && goimports -w .

vet:
    cd server && go vet ./...

lint: vet

clean:
    rm -rf bin server/static/dist server/static/locales server/tmp server/var dist
    rm -f server/coverage.out server/coverage.html server/coverage-usecase.out
