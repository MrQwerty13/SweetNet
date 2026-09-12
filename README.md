# SweetNet

Закрытая социальная сеть для одного круга друзей: приглашения, вход, общая лента,
текст и фотографии, редактирование своих постов, профиль и управление участниками.
Все активные участники видят все посты, включая историю до вступления.

Стек: **PostgreSQL → Go/chi/pgx → REST/JSON/OpenAPI → React/TypeScript/Vite**.
API и frontend обслуживает один Go-процесс. Node нужен только для сборки и разработки.

Текущая реализация находится в `codex/04-release`. Для ревью начните с
[карты проекта и веток](docs/REVIEW.md). [Проверки и ограничения](docs/PROGRESS.md)
разделяют подтверждённую локальную работу и ещё не проверенное развёртывание.

## Запуск без Docker

Нужны Go 1.26+, Node.js 22.12+ (проверено на 26.0.0), PostgreSQL 17 и отдельная
пустая база для SweetNet. Создайте её в своей PostgreSQL через pgAdmin, DataGrip
или `createdb`; учётная запись подключения должна иметь право применять миграции.
Команды выполняются из корня проекта.

```sh
make deps build
cp deploy/native.env.example .env.native
chmod 600 .env.native
```

В редакторе заполните `DATABASE_URL` в `.env.native` строкой подключения к своей
базе. Формат: `postgres://USER:PASSWORD@HOST:PORT/sweetnet?sslmode=disable`.
Специальные символы имени/пароля в URL кодируются; `sslmode=disable` подходит
только для локальной БД. Сохраните кавычки вокруг значения, не публикуйте файл.

```sh
set -a
. ./.env.native
set +a
.local/admin migrate
.local/admin bootstrap --username owner --display-name 'Владелец'
.local/server
```

CLI запросит пароль владельца скрыто: 12–128 символов. Откройте
[SweetNet на своём компьютере](http://127.0.0.1:8080), войдите, создайте ссылку
в разделе «Приглашения» и передайте её другу самостоятельно. Для доступа с других
устройств нужен сервер и HTTPS; localhost доступен только на этом компьютере.
Остановка сервера — Ctrl+C. Повторный запуск — `.local/server` с теми же env.

Для работы над интерфейсом вместо production-сборки запустите второй терминал:
`npm run dev --prefix web`. Vite работает на `http://localhost:5173` и проксирует
API на `http://127.0.0.1:8080`; для этого режима запустите Go с
`APP_ORIGIN=http://localhost:5173`. Основной путь проверки использует собранный
frontend и один origin на 8080.

## Запуск через Docker

Нужны Docker Engine/Desktop и Docker Compose v2.24+.

```sh
cp .env.example .env
chmod 600 .env
```

Задайте в `.env` случайный URL-safe `POSTGRES_PASSWORD` (например, random hex), затем:

```sh
docker compose --env-file .env -p sweetnet config --quiet
docker compose --env-file .env -p sweetnet up -d --build --wait --wait-timeout 180
docker compose --env-file .env -p sweetnet exec app /app/admin bootstrap --username owner --display-name 'Владелец'
```

Откройте [локальный SweetNet](http://localhost:8080). Остановка с сохранением
данных: `docker compose --env-file .env -p sweetnet stop`. Для обновления не
удаляйте volumes. Docker-сборка и полное восстановление контейнеров включены
в CI, но локально ещё не запускались: на машине реализации нет Docker Engine.

## Проверки

После `make deps` настройте `TEST_DATABASE_URL` на **отдельную** PostgreSQL-базу,
имя которой заканчивается `_test`. Тесты создают уникальные схемы; rehearsal
backup/restore создаёт две новые временные базы и требует право `CREATEDB`.
Не используйте базу с пользовательскими данными.

```sh
# Установить тестовый Chromium один раз; Docker не требуется.
cd tests && npx playwright install chromium && cd ..
# Экспортировать TEST_DATABASE_URL из приватного файла/настроек окружения, затем:
make check
```

`make check` проверяет форматирование, Go vet/race-тесты, frontend lint/build,
OpenAPI, скрипты обслуживания, четыре браузерных сценария и реальный
`pg_dump`/`pg_restore`. Для последнего нужны клиенты PostgreSQL 17 (`pg_dump`,
`pg_restore`) и `tar`. Альтернатива установленному Chromium — задать
`E2E_CHROME_PATH` на исполняемый файл Chrome.

Отдельные команды: `make integration`, `make browser`, `make contract`,
`make maintenance`, `make backup-rehearsal`. `go test ./...` без
`TEST_DATABASE_URL` явно пропускает интеграционные тесты и не считается полной проверкой.
Полная проверка контейнеров и Docker-скриптов: `node tests/verify-compose.mjs`.
Она создаёт отдельные случайно названные тестовые проекты, затем удаляет только их ресурсы.

## Обслуживание

Сброс забытого пароля — локальная команда `admin reset-password --username friend`;
пароль вводится скрыто, все сессии пользователя отзываются. В Docker используйте
`docker compose --env-file .env -p sweetnet exec app /app/admin ...`.

[Инструкция эксплуатации](docs/DEPLOYMENT.md) описывает HTTPS, согласованную
резервную копию БД и uploads, восстановление в новое окружение и обновления.
Для native-запуска остановите Go на время копирования БД и каталога `UPLOAD_DIR`;
проверенный пример восстановления находится в `tests/verify-backup.mjs`.

[OpenAPI](api/openapi.yaml) · [Архитектура и требования](docs/IMPLEMENTATION_PLAN.md)
· [Результаты проверок](docs/PROGRESS.md) · [Desktop](docs/design/feed-desktop.png)
· [Mobile](docs/design/feed-mobile.png)
