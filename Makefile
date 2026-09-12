.PHONY: deps build backend frontend format-check unit integration browser contract maintenance backup-rehearsal check

deps:
	cd backend && go mod download
	npm ci --prefix web
	npm ci --prefix tests

build: backend frontend

backend:
	mkdir -p .local
	cd backend && go build -o ../.local/server ./cmd/server
	cd backend && go build -o ../.local/admin ./cmd/admin

frontend:
	npm run build --prefix web

format-check:
	@test -z "$$(cd backend && gofmt -l cmd internal migrations)" || (echo 'Run gofmt on backend sources'; exit 1)
	npm run format:check --prefix web

unit:
	cd backend && go vet ./...
	cd backend && env -u TEST_DATABASE_URL go test ./...

integration:
	@test -n "$(TEST_DATABASE_URL)" || (echo 'Set TEST_DATABASE_URL to a database ending _test'; exit 1)
	cd backend && go test -race -count=1 ./...

browser: build
	@test -n "$(TEST_DATABASE_URL)" || (echo 'Set TEST_DATABASE_URL to a database ending _test'; exit 1)
	node tests/run-e2e.mjs

contract:
	npm run contract --prefix tests

maintenance:
	bash -n scripts/backup.sh scripts/restore.sh deploy/maintenance.sh
	python3 -m unittest discover -s deploy/tests -v

backup-rehearsal: backend
	node tests/verify-backup.mjs

check: format-check unit integration frontend contract maintenance browser backup-rehearsal
	npm run lint --prefix web
