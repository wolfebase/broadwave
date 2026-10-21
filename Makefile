.PHONY: help web server run dev test vet lint check build docker clean tokens docs

CONFIG ?= data
ADDR ?= :8477

help:
	@echo "make run      build the web app, then run the server on $(ADDR) with ./$(CONFIG)"
	@echo "make dev      run the server with -dev; pair with 'npm run dev' in web/"
	@echo "make test     go test + web typecheck"
	@echo "make lint     gofmt, go vet, and eslint"
	@echo "make check    everything CI runs before a push: test + lint + web budgets + API drift + fake-tuner relay smoke"
	@echo "make build    web + server binary in bin/"
	@echo "make docker   build the container image"
	@echo "make tokens   regenerate CSS (and the Apple theme, when apple/ is checked out) from design/tokens.json"
	@echo "make docs     write the static docs site to docs/sitegen/dist"

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
	cd web && node --experimental-strip-types --test storage.test.ts art.test.ts compat.test.ts switcher.test.ts sync.test.ts guide.test.ts remote.test.ts outage.test.ts resume.test.ts events.test.ts extras.test.ts captions.test.ts quiet.test.ts download.test.ts health.test.ts clear.test.ts seen.test.ts deviceCard.test.ts focusCycle.test.ts library.test.ts breaks.test.ts ends.test.ts startover.test.ts share.test.ts motion.test.ts dialogFocus.test.ts syncStatus.test.ts matchup.test.ts actionName.test.ts menuStep.test.ts contrast.test.ts optionArrow.test.ts seek.test.ts picture.test.ts filePlay.test.ts latest.test.ts search.test.ts keys.test.ts sourcePoll.test.ts plan.test.ts follow.test.ts lineup.test.ts scores.test.ts fetchNote.test.ts lookHits.test.ts settingsRace.test.ts golive.test.ts livepref.test.ts zoom.test.ts pairing.test.ts playlistKeep.test.ts recordAgain.test.ts storagePlanCopy.test.ts theme.test.ts playbackURL.test.ts cast.test.ts castLink.test.ts focusStep.test.ts install.test.ts tvKeys.test.ts tileArrow.test.ts bufferHole.test.ts deviceToken.test.ts format.test.ts moved.test.ts pairShare.test.ts pauseHold.test.ts playerChoice.test.ts recent.test.ts roomStart.test.ts seekLead.test.ts settle.test.ts snapshot.test.ts sounds.test.ts textField.test.ts time.test.ts mosaic.test.ts apiRequest.test.ts guideLogic.test.ts liveDelay.test.ts pictureLive.test.ts artLayout.test.ts guideWindow.test.ts guidePrefs.test.ts searchKey.test.ts guideKeys.test.ts gestures.test.ts warmPlan.test.ts dataVersions.test.ts homeRails.test.ts searchGroups.test.ts handoff.test.ts captionStyle.test.ts cloudflare.test.ts lineupModel.test.ts offlineCopy.test.ts scheduleView.test.ts manualTimed.test.ts setupSummary.test.ts sportsKind.test.ts serverZone.test.ts diagLines.test.ts logoTone.test.ts zone.test.ts i18n.test.ts i18nOverflow.test.ts
	cd web && npx vitest run
	cd docs/sitegen && go test ./...
	node --test scripts/soak/analyze.test.mjs scripts/soak/hls.test.mjs scripts/soak/sample.test.mjs scripts/soak/launch.test.mjs scripts/soak/browser.test.mjs

vet:
	go vet ./server/...

lint: web/node_modules
	scripts/check-names.sh
	@test -z "$$(gofmt -l server docs/sitegen)" || { gofmt -l server docs/sitegen; echo "run: gofmt -w server docs/sitegen"; exit 1; }
	go vet ./server/...
	cd web && npm run lint

check: test lint web
	scripts/bundle-budget.sh
	scripts/home-paint.sh
	go run ./server/cmd/apigen -check
	FAKE=1 scripts/relay-smoke.sh

build: web server

docker:
	docker build -f deploy/docker/Dockerfile -t broadwave .

clean:
	rm -rf bin server/cmd/broadwave/assets/web

tokens:
	node design/build.mjs

docs:
	cd docs/sitegen && go run . -out dist
