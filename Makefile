BINARY := paw
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X github.com/gongahkia/paw/cmd.version=$(VERSION) -X github.com/gongahkia/paw/cmd.gitCommit=$(COMMIT)
BENCH_DATASET ?= terminal-bench@2.0
BENCH_SWEBENCH_DATASET ?=
BENCH_MODEL ?= openai/glm-4.6
BENCH_JOBS_DIR ?= .paw/bench-jobs
BENCH_N_CONCURRENT ?= 4

.PHONY: build build-linux test lint fmt release-snapshot bench-smoke bench-oracle bench-full bench-swebench clean

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

build-linux:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS) -s -w" -o bin/paw-linux-amd64 .

test:
	go test ./... -race

lint:
	golangci-lint run

fmt:
	gofmt -w .

release-snapshot:
	goreleaser check
	goreleaser release --snapshot --clean --skip=publish

bench-smoke: build-linux
	go run . bench --config raw --dataset $(BENCH_DATASET) --model $(BENCH_MODEL) --jobs-dir $(BENCH_JOBS_DIR) --n-concurrent $(BENCH_N_CONCURRENT) --n-tasks 5 --job-name paw-smoke-raw
	go run . bench --config full --dataset $(BENCH_DATASET) --model $(BENCH_MODEL) --jobs-dir $(BENCH_JOBS_DIR) --n-concurrent $(BENCH_N_CONCURRENT) --n-tasks 5 --job-name paw-smoke-full

bench-oracle:
	harbor run --dataset $(BENCH_DATASET) --agent oracle --jobs-dir $(BENCH_JOBS_DIR) --job-name paw-oracle --n-tasks 1

bench-full: build-linux
	go run . bench --config raw --dataset $(BENCH_DATASET) --model $(BENCH_MODEL) --jobs-dir $(BENCH_JOBS_DIR) --n-concurrent $(BENCH_N_CONCURRENT) --n-tasks 89 --job-name paw-full-raw
	go run . bench --config no-compress --dataset $(BENCH_DATASET) --model $(BENCH_MODEL) --jobs-dir $(BENCH_JOBS_DIR) --n-concurrent $(BENCH_N_CONCURRENT) --n-tasks 89 --job-name paw-full-no-compress
	go run . bench --config full --dataset $(BENCH_DATASET) --model $(BENCH_MODEL) --jobs-dir $(BENCH_JOBS_DIR) --n-concurrent $(BENCH_N_CONCURRENT) --n-tasks 89 --job-name paw-full-full

bench-swebench: build-linux
	test -n "$(BENCH_SWEBENCH_DATASET)" || (echo "set BENCH_SWEBENCH_DATASET to the Harbor SWE-bench Verified dataset id" >&2; exit 2)
	go run . bench --config raw --dataset $(BENCH_SWEBENCH_DATASET) --model $(BENCH_MODEL) --jobs-dir $(BENCH_JOBS_DIR) --n-concurrent $(BENCH_N_CONCURRENT) --job-name paw-swebench-raw
	go run . bench --config full --dataset $(BENCH_SWEBENCH_DATASET) --model $(BENCH_MODEL) --jobs-dir $(BENCH_JOBS_DIR) --n-concurrent $(BENCH_N_CONCURRENT) --job-name paw-swebench-full

clean:
	rm -rf bin coverage.out dist
