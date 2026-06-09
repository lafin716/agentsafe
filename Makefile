.PHONY: build-cli build-desktop dev-desktop deploy-cli lint

build-cli:
	go build -o agr ./apps/cli

deploy-cli:
	sudo ./scripts/deploy-cli.sh

build-desktop:
	./scripts/build-desktop.sh

dev-desktop:
	./scripts/dev-desktop.sh

lint:
	go vet ./...
