.PHONY: all build build-agent proto test clean frontend dev-backend dev-frontend

all: frontend build-agent build

proto:
	protoc --go_out=. --go_opt=module=easy42 proto/agent.proto

build-agent:
	mkdir -p internal/agent/embedded bin
	cd agent && cargo build --release --target x86_64-unknown-linux-musl
	cp agent/target/x86_64-unknown-linux-musl/release/easy42-agent internal/agent/embedded/easy42-agent-x86_64
	cp agent/target/x86_64-unknown-linux-musl/release/easy42-agent bin/easy42-agent
	cd agent && CC_aarch64_unknown_linux_musl=aarch64-linux-gnu-gcc RUSTFLAGS="-C linker=aarch64-linux-gnu-gcc" cargo build --release --target aarch64-unknown-linux-musl
	cp agent/target/aarch64-unknown-linux-musl/release/easy42-agent internal/agent/embedded/easy42-agent-aarch64
	cd agent && CC_armv7_unknown_linux_musleabihf=arm-linux-gnueabihf-gcc RUSTFLAGS="-C linker=arm-linux-gnueabihf-gcc" cargo build --release --target armv7-unknown-linux-musleabihf
	cp agent/target/armv7-unknown-linux-musleabihf/release/easy42-agent internal/agent/embedded/easy42-agent-armv7
	cd agent && CC_mipsel_unknown_linux_musl=mipsel-linux-gnu-gcc RUSTFLAGS="-C linker=mipsel-linux-gnu-gcc -C link-arg=-lm" RUSTC_BOOTSTRAP=1 cargo build --release -Z build-std=std,panic_abort --target mipsel-unknown-linux-musl
	cp agent/target/mipsel-unknown-linux-musl/release/easy42-agent internal/agent/embedded/easy42-agent-mipsel

build: build-agent
	go build -trimpath -ldflags "-s -w" -o bin/easy42 main.go

test:
	go test -v ./internal/...

dev-backend:
	go run main.go serve --listen 127.0.0.1:4242

dev-frontend:
	cd web && npm run dev

clean:
	rm -rf bin/ web/dist
