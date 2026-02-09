APP=clidojo
BIN=bin/$(APP)

.PHONY: build run test fmt dev-web dev-run snapshots image webterm webterm-restart webterm-mcp e2e e2e-update

build:
	go build -o $(BIN) ./cmd/clidojo

run: build
	./$(BIN)

dev-run: build
	./$(BIN) --dev --sandbox=mock --demo=playable

dev-web:
	./scripts/dev-web.sh

webterm: build
	chmod +x ./scripts/webterm.sh
	./scripts/webterm.sh

webterm-restart: build
	chmod +x ./scripts/webterm-restart.sh
	./scripts/webterm-restart.sh

webterm-mcp: build
	chmod +x ./scripts/webterm-mcp.sh
	./scripts/webterm-mcp.sh

test:
	go test ./...

fmt:
	gofmt -w $(shell rg --files -g '*.go')

snapshots:
	./scripts/dev-snapshots.sh

e2e:
	cd e2e/playwright && corepack pnpm test

e2e-update:
	cd e2e/playwright && corepack pnpm run update-snapshots

image:
	docker build -t clidojo/builtin-core:0.1.0 -f packs/builtin-core/image/Dockerfile packs/builtin-core/image
