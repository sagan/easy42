.PHONY: all build build-agent proto test clean frontend dev-backend dev-frontend

all: frontend build-agent build

proto:
	protoc --go_out=. --go_opt=module=easy42 proto/agent.proto

build-agent:
	mkdir -p internal/agent/embedded bin
	cross build --manifest-path agent/Cargo.toml --target-dir agent/target/x86_64 --release --target x86_64-unknown-linux-musl
	cp agent/target/x86_64/x86_64-unknown-linux-musl/release/easy42-agent internal/agent/embedded/easy42-agent-x86_64
	cp agent/target/x86_64/x86_64-unknown-linux-musl/release/easy42-agent bin/easy42-agent
	cross build --manifest-path agent/Cargo.toml --target-dir agent/target/aarch64 --release --target aarch64-unknown-linux-musl
	cp agent/target/aarch64/aarch64-unknown-linux-musl/release/easy42-agent internal/agent/embedded/easy42-agent-aarch64
	cross build --manifest-path agent/Cargo.toml --target-dir agent/target/armv7 --release --target armv7-unknown-linux-musleabihf
	cp agent/target/armv7/armv7-unknown-linux-musleabihf/release/easy42-agent internal/agent/embedded/easy42-agent-armv7
	cross +nightly build --manifest-path agent/Cargo.toml --target-dir agent/target/mipsel --release --target mipsel-unknown-linux-musl
	cp agent/target/mipsel/mipsel-unknown-linux-musl/release/easy42-agent internal/agent/embedded/easy42-agent-mipsel

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

build: build-agent
	go build -trimpath -ldflags "-s -w -X easy42/internal/version.Version=$(VERSION) -X easy42/internal/version.Commit=$(COMMIT) -X easy42/internal/version.Date=$(DATE) -X easy42/internal/version.BuiltBy=make" -o bin/easy42 main.go

test:
	go test -v ./internal/...

dev-backend:
	go run main.go serve --listen 0.0.0.0:4242

dev-frontend:
	cd web && npm run dev

clean:
	rm -rf bin/ web/dist
