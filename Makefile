.PHONY: help web server run dev test vet lint check build docker clean tokens apple apple-test

CONFIG ?= data
ADDR ?= :8477

help:
	@echo "make run      build the web app, then run the server on $(ADDR) with ./$(CONFIG)"
	@echo "make dev      run the server with -dev; pair with 'npm run dev' in web/"
	@echo "make test     go test + web typecheck"
	@echo "make lint     gofmt, go vet, eslint, swiftlint, and swiftformat"
	@echo "make check    everything CI runs before a push: test + lint + web budgets + API drift + fake-tuner relay smoke"
	@echo "make build    web + server binary in bin/"
	@echo "make docker   build the container image"
	@echo "make apple    generate the Xcode project and build the iOS and tvOS apps"
	@echo "make tokens   regenerate CSS and Swift from design/tokens.json"

web/node_modules: web/package-lock.json
	cd web && PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm ci
	@touch web/node_modules

web: web/node_modules
	cd web && npm run build

server:
	go build -o bin/broadwave ./server/cmd/broadwave

run: web
	go run ./server/cmd/broadwave -config $(CONFIG) -addr $(ADDR)

dev:
	go run ./server/cmd/broadwave -config $(CONFIG) -addr $(ADDR) -dev

test: web/node_modules
	go test ./server/...
	go test -race ./server/internal/live/ ./server/internal/hdhr/fake/
	cd web && npx tsc --noEmit
	cd web && node --experimental-strip-types --test storage.test.ts art.test.ts compat.test.ts switcher.test.ts sync.test.ts guide.test.ts remote.test.ts outage.test.ts resume.test.ts events.test.ts extras.test.ts captions.test.ts quiet.test.ts download.test.ts filePlay.test.ts health.test.ts clear.test.ts seen.test.ts deviceCard.test.ts focusCycle.test.ts library.test.ts breaks.test.ts ends.test.ts startover.test.ts latest.test.ts search.test.ts keys.test.ts sourcePoll.test.ts plan.test.ts follow.test.ts lineup.test.ts scores.test.ts

vet:
	go vet ./server/...

lint: web/node_modules
	scripts/check-names.sh
	@test -z "$$(gofmt -l server)" || { gofmt -l server; echo "run: gofmt -w server"; exit 1; }
	go vet ./server/...
	cd web && npm run lint
	swiftlint lint --strict
	swiftformat --lint .

check: test lint web
	scripts/bundle-budget.sh
	scripts/home-paint.sh
	cd apple/Packages/BroadwaveKit && swift test
	go run ./server/cmd/apigen -check
	FAKE=1 scripts/relay-smoke.sh

build: web server

docker:
	docker build -f deploy/docker/Dockerfile -t broadwave .

clean:
	rm -rf bin server/cmd/broadwave/assets/web

tokens:
	node design/build.mjs

apple:
	cd apple && xcodegen generate
	cd apple && xcodebuild -project Broadwave.xcodeproj -scheme Broadwave -destination 'generic/platform=iOS Simulator' -derivedDataPath build/dd CODE_SIGNING_ALLOWED=NO build-for-testing
	cd apple && xcodebuild -project Broadwave.xcodeproj -scheme BroadwaveTV -destination 'generic/platform=tvOS Simulator' -derivedDataPath build/dd CODE_SIGNING_ALLOWED=NO build-for-testing

apple-test:
	cd apple/Packages/BroadwaveKit && swift test
