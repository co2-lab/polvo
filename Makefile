.PHONY: build ui-dev app-dev tauri-dev clean dev web-dev tui

TARGET := $(shell rustc -Vv | grep host | cut -f2 -d' ')
BUILD_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y%m%d-%H%M%S)
LDFLAGS := -X main.CommitSHA=$(BUILD_SHA) -X main.BuildDate=$(BUILD_DATE)

.PHONY: kill-server
kill-server:
	@echo "→ killing polvo processes..."
	@pkill -9 -f "polvo" 2>/dev/null || true
	@pkill -9 -f "go-build.*/polvo" 2>/dev/null || true
	@lsof -ti :7373 | xargs kill -9 2>/dev/null || true
	@sleep 1
	@echo "→ port 7373: $$(lsof -ti :7373 | wc -l | tr -d ' ') processes remaining"

# Tauri app desktop (dev)
dev: kill-server
	cd app && go build -ldflags "$(LDFLAGS)" -o ../desktop/bin/polvo-$(TARGET) ./cmd/polvo
	mkdir -p desktop/target/debug
	cp desktop/bin/polvo-$(TARGET) desktop/target/debug/polvo
	cd desktop && POLVO_ROOT=$(PWD) npx tauri dev --config tauri.conf.json

# Desenvolvimento web (sem Tauri): Go server + Vite com proxy
web-dev: kill-server
	@echo "→ starting Go server (port 7373)..."
	POLVO_ROOT=$(PWD) go run -ldflags "$(LDFLAGS)" ./app/cmd/polvo &
	@echo "→ starting Vite dev server..."
	cd ui && npm run dev

build:
	cd ui && npm run build
	cd app && make build
	cd desktop && npx tauri build

ui-dev:
	cd ui && npm run dev

app-dev:
	go run ./app/cmd/polvo

# TUI interativo do agente (sem desktop)
tui:
	cd app && go build -ldflags "$(LDFLAGS)" -o ../bin/polvo ./cmd/polvo
	POLVO_ROOT=$(PWD) ./bin/polvo

tauri-dev:
	cd desktop && POLVO_ROOT=$(PWD) npx tauri dev --config tauri.conf.json

test:
	cd app && go test ./...

clean:
	rm -rf ui/dist
	cd app && make clean
	rm -rf desktop/target
