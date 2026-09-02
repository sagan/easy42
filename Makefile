.PHONY: all build test clean frontend dev-backend dev-frontend

all: frontend build

frontend:
	cd web && npm install && npm run build

build:
	go build -trimpath -ldflags "-s -w" -o bin/easy42 main.go

test:
	go test -v ./internal/...

dev-backend:
	go run main.go serve --listen 127.0.0.1:4242

dev-frontend:
	cd web && npm run dev

clean:
	rm -rf bin/ web/dist
