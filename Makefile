.PHONY: build ui-dev app-dev tauri-dev clean dev dev-reset web-dev tui generate

TARGET := $(shell rustc -Vv | grep host | cut -f2 -d' ')
BUILD_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y%m%d-%H%M%S)
LDFLAGS      := -X main.CommitSHA=$(BUILD_SHA) -X main.BuildDate=$(BUILD_DATE)
LDFLAGS_DEV  := $(LDFLAGS)
SEQ_URL      ?= http://localhost:5341

.PHONY: kill-server
kill-server:
	@echo "→ killing polvo processes..."
	@lsof -ti :7373 | xargs kill -9 2>/dev/null || true
	@pkill -9 -f "/bin/polvo" 2>/dev/null || true
	@pkill -9 -f "go-build.*/polvo" 2>/dev/null || true
	@for i in 1 2 3 4 5; do \
		if [ -z "$$(lsof -ti :7373)" ]; then break; fi; \
		sleep 0.5; \
	done
	@echo "→ port 7373: $$(lsof -ti :7373 | wc -l | tr -d ' ') processes remaining"

# In dev mode Tauri uses identifier "app"; in release it uses "io.co2lab.polvo"
WEBKIT_STORAGE_DEV     := $(HOME)/Library/WebKit/app/WebsiteData
WEBKIT_STORAGE_RELEASE := $(HOME)/Library/WebKit/io.co2lab.polvo/WebsiteData

dev: kill-server
	cd app && go build -ldflags "$(LDFLAGS_DEV)" -o ../desktop/bin/polvo-$(TARGET) ./cmd/polvo
	mkdir -p desktop/target/debug bin
	cp desktop/bin/polvo-$(TARGET) desktop/target/debug/polvo
	cp desktop/bin/polvo-$(TARGET) bin/polvo
	cd desktop && SEQ_URL=$(SEQ_URL) POLVO_ROOT=$(PWD) npx tauri dev --config tauri.conf.json

# Same as dev but wipes WebKit localStorage first (use when UI state is broken)
dev-reset: kill-server
	@echo "→ clearing WebKit storage..."
	@rm -rf "$(WEBKIT_STORAGE_DEV)" "$(WEBKIT_STORAGE_RELEASE)"
	cd app && go build -ldflags "$(LDFLAGS_DEV)" -o ../desktop/bin/polvo-$(TARGET) ./cmd/polvo
	mkdir -p desktop/target/debug bin
	cp desktop/bin/polvo-$(TARGET) desktop/target/debug/polvo
	cp desktop/bin/polvo-$(TARGET) bin/polvo
	cd desktop && SEQ_URL=$(SEQ_URL) POLVO_ROOT=$(PWD) npx tauri dev --config tauri.conf.json

# Desenvolvimento web (sem Tauri): Go server + Vite com proxy
web-dev: kill-server
	@echo "→ starting Go server (port 7373)..."
	SEQ_URL=$(SEQ_URL) POLVO_ROOT=$(PWD) go run -ldflags "$(LDFLAGS_DEV)" ./app/cmd/polvo &
	@echo "→ starting Vite dev server..."
	cd ui && npm run dev

generate:
	cd app && go generate ./internal/provider/...

build: generate
	cd ui && npm run build
	cd app && make build
	cd desktop && npx tauri build

ui-dev:
	cd ui && npm run dev

app-dev:
	go run ./app/cmd/polvo

tui: generate
	cd app && go build -ldflags "$(LDFLAGS_DEV)" -o ../bin/polvo ./cmd/polvo
	SEQ_URL=$(SEQ_URL) POLVO_ROOT=$(PWD) ./bin/polvo

tauri-dev:
	cd desktop && POLVO_ROOT=$(PWD) npx tauri dev --config tauri.conf.json

test:
	cd app && go test ./...

clean:
	rm -rf ui/dist
	cd app && make clean
	rm -rf desktop/target

