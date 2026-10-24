.PHONY: all build test test-ipc clean

GOTMPDIR ?= $(HOME)/.cache/tmp
BINARY_NAME = m3tal-compose
SRC_DIR = src
BIN_DIR = bin

all: build

build:
	@mkdir -p $(GOTMPDIR) $(BIN_DIR)
	@echo "🔨 Compiling $(BINARY_NAME)..."
	@cd $(SRC_DIR) && GOTMPDIR=$(GOTMPDIR) go build -ldflags="-s -w" -o ../$(BIN_DIR)/$(BINARY_NAME) .
	@echo "✅ Built $(BIN_DIR)/$(BINARY_NAME)"

test:
	@mkdir -p $(GOTMPDIR)
	@echo "🧪 Running unit tests..."
	@cd $(SRC_DIR) && GOTMPDIR=$(GOTMPDIR) go test -v ./...

test-ipc: build
	@echo "🔌 Testing JSON-RPC 2.0 stdio handshake..."
	@INIT_RESP=$$(echo '{"jsonrpc":"2.0","id":1,"method":"m3tal.init"}' | ./$(BIN_DIR)/$(BINARY_NAME)); \
	if echo "$$INIT_RESP" | grep -q '"name":"compose"'; then \
		echo "  ✓ Handshake (m3tal.init): SUCCESS"; \
	else \
		echo "  ✗ Handshake (m3tal.init): FAILED ($$INIT_RESP)"; exit 1; \
	fi
	@echo "📊 Testing telemetry collection..."
	@COLLECT_RESP=$$(echo '{"jsonrpc":"2.0","id":2,"method":"m3tal.collect"}' | ./$(BIN_DIR)/$(BINARY_NAME)); \
	if echo "$$COLLECT_RESP" | grep -q '"status":"ok"'; then \
		echo "  ✓ Collector (m3tal.collect): SUCCESS"; \
	else \
		echo "  ✗ Collector (m3tal.collect): FAILED ($$COLLECT_RESP)"; exit 1; \
	fi
	@echo "⚡ Testing action dispatch..."
	@ACTION_RESP=$$(echo '{"jsonrpc":"2.0","id":4,"method":"m3tal.action","params":{"action_id":"validate_yaml","content":"services: { app: { image: redis } }"}}' | ./$(BIN_DIR)/$(BINARY_NAME)); \
	if echo "$$ACTION_RESP" | grep -q '"valid":true'; then \
		echo "  ✓ Action (m3tal.action): SUCCESS"; \
	else \
		echo "  ✗ Action (m3tal.action): FAILED ($$ACTION_RESP)"; exit 1; \
	fi
	@echo "🛑 Testing graceful shutdown..."
	@SHUTDOWN_RESP=$$(echo '{"jsonrpc":"2.0","id":5,"method":"m3tal.shutdown"}' | ./$(BIN_DIR)/$(BINARY_NAME)); \
	if echo "$$SHUTDOWN_RESP" | grep -q '"status":"ok"'; then \
		echo "  ✓ Shutdown (m3tal.shutdown): SUCCESS"; \
	else \
		echo "  ✗ Shutdown (m3tal.shutdown): FAILED ($$SHUTDOWN_RESP)"; exit 1; \
	fi
	@echo "✅ All IPC tests passed!"

clean:
	@rm -rf $(BIN_DIR)
