.PHONY: help build-all test-all build test audit audit-all package package-all bump bump-check build-changed package-changed clean

help:
	@echo "M3TAL Plugins Monorepo Build Targets:"
	@echo "  make bump [PLUGIN=<name>]   - Smart auto-bump version and CHANGELOG for touched plugins"
	@echo "  make bump-check             - Dry-run inspect which plugins are touched/need bump"
	@echo "  make audit [PLUGIN=<name>]  - Audit plugin compliance against 6 standardization gates"
	@echo "  make audit-all              - Audit all plugins in the repository"
	@echo "  make build PLUGIN=<name>    - Build backend and UI for specified plugin"
	@echo "  make build-changed          - Build only plugins that have been modified"
	@echo "  make build-all              - Build all plugins in the repository"
	@echo "  make test PLUGIN=<name>     - Run tests for specified plugin"
	@echo "  make test-all               - Run tests across all plugins"
	@echo "  make package PLUGIN=<name>  - Build Debian package (.deb) for specified plugin"
	@echo "  make package-changed        - Build Debian packages only for modified plugins"
	@echo "  make package-all            - Build Debian packages for all plugins"
	@echo "  make clean                  - Clean build artifacts across all plugins"

BASE_ARG = $(if $(BASE),--base "$(BASE)",)

bump-check:
	@./scripts/smart-bump.sh $(BASE_ARG)

bump:
ifdef PLUGIN
	@./scripts/smart-bump.sh --plugin "$(PLUGIN)" $(BASE_ARG) --apply
else
	@./scripts/smart-bump.sh $(BASE_ARG) --apply
endif

audit:
ifndef PLUGIN
	$(error PLUGIN is not set. Usage: make audit PLUGIN=<plugin-name>)
endif
	@./scripts/audit-plugin.sh "$(PLUGIN)"

audit-all:
	@for p in $$(find . -maxdepth 1 -mindepth 1 -type d ! -name '.*' ! -name 'scripts'); do \
		if [ -f "$$p/plugin.yaml" ]; then \
			./scripts/audit-plugin.sh "$$(basename "$$p")"; \
		fi; \
	done

build-changed:
	@TOUCHED=$$("./scripts/smart-bump.sh" $(BASE_ARG) --list-touched); \
	if [ -z "$$TOUCHED" ]; then \
		echo "ℹ️ No plugins have been modified. Nothing to build."; \
	else \
		for p in $$TOUCHED; do \
			$(MAKE) build PLUGIN="$$p"; \
		done; \
	fi

package-changed:
	@TOUCHED=$$("./scripts/smart-bump.sh" $(BASE_ARG) --list-touched); \
	if [ -z "$$TOUCHED" ]; then \
		echo "ℹ️ No plugins have been modified. Nothing to package."; \
	else \
		for p in $$TOUCHED; do \
			$(MAKE) package PLUGIN="$$p"; \
		done; \
	fi

build:
ifndef PLUGIN
	$(error PLUGIN is not set. Usage: make build PLUGIN=<plugin-name>)
endif
	@echo "🔨 Building plugin: $(PLUGIN)..."
	@if [ -f "$(PLUGIN)/Makefile" ]; then \
		$(MAKE) -C "$(PLUGIN)" build; \
	elif [ -f "$(PLUGIN)/plugin.yaml" ]; then \
		echo "Building standard plugin in $(PLUGIN)..."; \
		[ -d "$(PLUGIN)/ui" ] && (cd "$(PLUGIN)/ui" && npm run build 2>/dev/null || true); \
		[ -d "$(PLUGIN)/src" ] && (cd "$(PLUGIN)/src" && go build -o ../bin/$(PLUGIN) . 2>/dev/null || true); \
	else \
		echo "Plugin $(PLUGIN) not found or missing plugin.yaml"; exit 1; \
	fi

build-all:
	@for p in $$(find . -maxdepth 1 -mindepth 1 -type d ! -name '.*' ! -name 'scripts'); do \
		if [ -f "$$p/plugin.yaml" ]; then \
			$(MAKE) build PLUGIN="$$(basename "$$p")"; \
		fi; \
	done

test:
ifndef PLUGIN
	$(error PLUGIN is not set. Usage: make test PLUGIN=<plugin-name>)
endif
	@if [ -f "$(PLUGIN)/Makefile" ]; then \
		$(MAKE) -C "$(PLUGIN)" test; \
	fi

test-all:
	@for p in $$(find . -maxdepth 1 -mindepth 1 -type d ! -name '.*' ! -name 'scripts'); do \
		if [ -f "$$p/plugin.yaml" ]; then \
			$(MAKE) test PLUGIN="$$(basename "$$p")"; \
		fi; \
	done

package:
ifndef PLUGIN
	$(error PLUGIN is not set. Usage: make package PLUGIN=<plugin-name>)
endif
	@if [ "$(PLUGIN)" = "m3tal-mesh-cluster" ] || [ "$(PLUGIN)" = "cluster" ]; then \
		./scripts/build-cluster-deb.sh; \
	else \
		./scripts/build-plugin-deb.sh "$(PLUGIN)"; \
	fi

package-all:
	@for p in $$(find . -maxdepth 1 -mindepth 1 -type d ! -name '.*' ! -name 'scripts'); do \
		if [ -f "$$p/plugin.yaml" ]; then \
			./scripts/build-plugin-deb.sh "$$(basename "$$p")"; \
		fi; \
	done
	@if [ -f "scripts/build-cluster-deb.sh" ]; then \
		./scripts/build-cluster-deb.sh; \
	fi

clean:
	@rm -f *.deb *.sha256
	@for p in $$(find . -maxdepth 1 -mindepth 1 -type d ! -name '.*' ! -name 'scripts'); do \
		if [ -f "$$p/Makefile" ]; then \
			$(MAKE) -C "$$p" clean 2>/dev/null || true; \
		fi; \
	done
