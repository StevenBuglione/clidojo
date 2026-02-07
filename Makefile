APP=clidojo
BIN=bin/$(APP)

.PHONY: build run test fmt dev-web dev-run snapshots image

build:
	go build -o $(BIN) ./cmd/clidojo

run: build
	./$(BIN)

dev-run: build
	./$(BIN) --dev --sandbox=mock --demo=playable

dev-web:
	./scripts/dev-web.sh

test:
	go test ./...

fmt:
	gofmt -w $(shell rg --files -g '*.go')

snapshots:
	./scripts/dev-snapshots.sh

image:
	docker build -t clidojo/builtin-core:0.1.0 -f packs/builtin-core/image/Dockerfile packs/builtin-core/image
