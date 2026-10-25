.PHONY: build frontend frontend-test test race vet install uninstall demo
build:
	./build.sh
frontend:
	node web/build.mjs
frontend-test:
	node --test web/test/*.test.mjs
test:
	go test -count=1 ./...
race:
	go test -race -count=1 ./...
vet:
	go vet ./...
install:
	@echo "RC: complete UPGRADE.md acceptance gates before explicitly installing the verified binary."
	@exit 1
uninstall:
	sudo /usr/local/bin/scyllascope service uninstall
demo:
	go run ./cmd/devserver
