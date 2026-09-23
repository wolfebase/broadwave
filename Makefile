.PHONY: help web server run dev test vet lint check build docker clean tokens apple apple-test

CONFIG ?= data
ADDR ?= :8477

help:
	@echo "make run      build the web app, then run the server on $(ADDR) with ./$(CONFIG)"
	@echo "make dev      run the server with -dev; pair with 'npm run dev' in web/"
	@echo "make test     go test + web typecheck"
	@echo "make lint     eslint, swiftlint, and swiftformat"
	@echo "make check    everything CI runs before a push: test + lint"
	@echo "make build    web + server binary in bin/"
	@echo "make docker   build the container image"
	@echo "make apple    generate the Xcode project and build the iOS and tvOS apps"
	@echo "make tokens   regenerate CSS and Swift from design/tokens.json"

web/node_modules: web/package-lock.json
	cd web && npm ci
	@touch web/node_modules

web: web/node_modules
	cd web && npm run build

server:
	go build -o bin/waveguide ./server/cmd/waveguide

run: web
	go run ./server/cmd/waveguide -config $(CONFIG) -addr $(ADDR)

dev:
	go run ./server/cmd/waveguide -config $(CONFIG) -addr $(ADDR) -dev

test: web/node_modules
	go test ./server/...
	cd web && npx tsc --noEmit

vet:
	go vet ./server/...

lint: web/node_modules
	cd web && npm run lint
	swiftlint lint --strict
	swiftformat --lint .

check: test lint

build: web server

docker:
	docker build -f deploy/docker/Dockerfile -t waveguide .

clean:
	rm -rf bin server/cmd/waveguide/assets/web

tokens:
	node design/build.mjs

apple:
	cd apple && xcodegen generate
	cd apple && xcodebuild -project Waveguide.xcodeproj -scheme Waveguide -destination 'generic/platform=iOS Simulator' -derivedDataPath build/dd CODE_SIGNING_ALLOWED=NO build
	cd apple && xcodebuild -project Waveguide.xcodeproj -scheme WaveguideTV -destination 'generic/platform=tvOS Simulator' -derivedDataPath build/dd CODE_SIGNING_ALLOWED=NO build

apple-test:
	cd apple/Packages/OTAKit && swift test
