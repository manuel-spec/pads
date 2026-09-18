.PHONY: build test test-race fmt lint clean extension extension-check

BINARY := pads
EXT_SRC := extension/src
EXT_DIST := dist/extension
EXT_TARGETS := chrome firefox

build:
	go build -o $(BINARY) .

test:
	go test ./...

test-race:
	go test -race ./...

fmt:
	gofmt -w .
	goimports -w .

# extension assembles a loadable directory per browser. The sources are shared;
# only the manifest differs, so each target is the same tree with its own
# manifest.json dropped in.
extension:
	@for target in $(EXT_TARGETS); do \
		rm -rf $(EXT_DIST)/$$target; \
		mkdir -p $(EXT_DIST)/$$target; \
		cp -R $(EXT_SRC)/. $(EXT_DIST)/$$target/; \
		cp extension/manifest.$$target.json $(EXT_DIST)/$$target/manifest.json; \
		echo "built $(EXT_DIST)/$$target"; \
	done

# extension-check loads each browser's background entry under stub APIs. A
# syntax check does not catch a background script that loads but registers
# nothing. Skipped when node is unavailable.
extension-check:
	@if command -v node >/dev/null 2>&1; then \
		node extension/check.mjs; \
	else \
		echo "node not found; skipping extension check"; \
	fi

clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf dist
