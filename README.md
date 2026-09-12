# SweetNet

Закрытая социальная сеть для одного круга: владелец приглашает друзей, а публикации доступны только активным участникам.

## Локальный запуск через Docker

Нужны Docker Compose v2.24+ и Git. Из корня репозитория:

```sh
cp .env.example .env
chmod 600 .env
```

Задайте в `.env` случайный `POSTGRES_PASSWORD`, затем:

```sh
docker compose --env-file .env -p sweetnet config --quiet
docker compose --env-file .env -p sweetnet up -d --build --wait --wait-timeout 180
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/readyz
docker compose --env-file .env -p sweetnet exec app /app/admin bootstrap --username owner --display-name 'Владелец'
```

Пароль владельца вводится скрыто в CLI. После входа владелец создаёт приглашение в разделе «Приглашения» и передаёт ссылку другу. Остановка без удаления данных: `docker compose --env-file .env -p sweetnet stop`. Подробные инструкции HTTPS, backup и restore находятся в [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Проверки

```sh
cd backend && gofmt -w ./cmd ./internal ./migrations && go vet ./... && go test ./...
cd ../web && npm ci && npm run typecheck && npm run lint && npm run build
cd ../tests && npm ci && node validate-openapi.mjs
```

Интеграционные тесты используют отдельную PostgreSQL и переменную `TEST_DATABASE_URL`; браузерный сценарий запускается через `node tests/run-e2e.mjs`. Точные результаты и непроведённые проверки ведутся в [docs/PROGRESS.md](docs/PROGRESS.md).

Архитектура и критерии релиза описаны в [docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md), API-контракт — в [api/openapi.yaml](api/openapi.yaml).
