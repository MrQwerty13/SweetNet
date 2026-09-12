# Запуск и эксплуатация SweetNet

## Что подготовлено и что проверено

Один Go-процесс обслуживает API и собранный frontend. PostgreSQL и фотографии
хранятся в отдельных именованных Docker volumes. PostgreSQL не публикует порт
на хосте; приложение доступно на `127.0.0.1:8080`. Сначала Compose ждёт здоровую
БД, затем успешное завершение `admin migrate`, затем запускает приложение.

Образы зафиксированы по версии: Go `1.26.4-alpine3.23`, Node
`26.0.0-bookworm-slim`, runtime Alpine `3.23.3`, PostgreSQL
`17.11-alpine3.23`, Caddy `2.11.4-alpine`. Go и Node согласованы с локальным
контрактом сборки. Runtime работает как `10001:10001`, с read-only rootfs,
без Linux capabilities, с отдельными uploads и временным `/tmp`.
Node отсутствует в итоговом образе; миграции встроены в `admin`.

Эта конфигурация подготовлена 2026-09-12. Docker daemon в среде реализации
недоступен: сборка образов, их загрузка из registry, запуск контейнеров,
полное восстановление и HTTPS на реальном домене пока **не проверены**.
Синтаксическая проверка Compose не заменяет эти проверки. Точные результаты
локальной проверки приведены в конце документа.

Официальные справочники: [порядок запуска Docker Compose](https://docs.docker.com/compose/how-tos/startup-order/),
[Go 1.26](https://go.dev/doc/go1.26), [образы PostgreSQL 17](https://hub.docker.com/_/postgres/tags?name=17.&page=1),
[образ Caddy](https://hub.docker.com/_/caddy). Теги фиксируют версии, но могут
пересобираться поставщиком. Перед реальным запуском проверьте доступность
образов для архитектуры VPS; для неизменяемого развёртывания сохраните digest
каждого образа и используйте digest или проверенный сохранённый образ.

## Первый локальный запуск

Нужны Git и Docker с Compose v2.24 или новее (v5 также подходит). Для скриптов
backup/restore дополнительно нужны Bash 3.2+ и Python 3.9+ на хосте.
Команды ниже выполняются из корня репозитория.

```sh
cp .env.example .env
chmod 600 .env
```

В локальном редакторе заполните `POSTGRES_PASSWORD` случайным значением,
например результатом `openssl rand -hex 32`, запущенного в личном терминале.
Не вставляйте пароль в команды, сообщения, git или логи. Стандартного пароля
нет: Compose откажется запускаться с пустой переменной.

`POSTGRES_USER` и `POSTGRES_DB` состоят из ASCII-букв, цифр и `_`.
Пароль БД для этой Compose-конфигурации состоит только из URL-safe символов
`A–Z a–z 0–9 . _ ~ -`, поскольку он подставляется в `DATABASE_URL`; random hex
подходит. Для другого набора символов сначала добавьте безопасное URL-кодирование
в конфигурацию. Не подставляйте percent-encoded пароль в `POSTGRES_PASSWORD`:
это изменит пароль самой БД. Не меняйте пароль уже созданной БД одним изменением
`.env`: Docker init-переменные действуют только при первом создании volume.

Локальные параметры:

```dotenv
APP_ENV=development
APP_ORIGIN=http://localhost:8080
APP_PORT=8080
POSTGRES_USER=sweetnet
POSTGRES_DB=sweetnet
SWEETNET_IMAGE=sweetnet:local
```

В контейнере Compose задаёт `DATABASE_URL` на сервис `db`, `HTTP_ADDR=:8080`,
`UPLOAD_DIR=/data/uploads`, `WEB_DIR=/app/web`. Хостовые значения этих четырёх
переменных намеренно не меняют контейнерные пути и адрес БД.
`APP_PORT` меняет только localhost-порт; `APP_ORIGIN` должен ему соответствовать.
`.env` не включается в образ. Не используйте `docker compose config` без
`--quiet`: полный вывод раскрывает пароль подключения.

```sh
docker compose --env-file .env -p sweetnet config --quiet
docker compose --env-file .env -p sweetnet up -d --build --wait --wait-timeout 180
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/readyz
docker compose --env-file .env -p sweetnet exec app /app/admin bootstrap --username owner --display-name 'Владелец'
```

Пароль владельца вводится скрыто по приглашению CLI. Не передавайте его
аргументом команды или через `echo`; для интерактивного ввода не добавляйте
`-T`. Повторное создание владельца должно быть отклонено backend.
Откройте [локальный SweetNet](http://localhost:8080), войдите, создайте
приглашение в управлении участниками и передайте ссылку другу самостоятельно.

```sh
# Остановка с сохранением данных.
docker compose --env-file .env -p sweetnet stop
# Возобновление существующих контейнеров.
docker compose --env-file .env -p sweetnet start
# Сброс пароля: CLI запросит новый пароль скрыто и отзовёт сессии.
docker compose --env-file .env -p sweetnet exec app /app/admin reset-password --username friend
```

Не используйте `down -v`, `docker volume prune` и удаление volumes для
обновления, восстановления или устранения ошибки. Обычный `down` сохраняет
volumes, но удаляет контейнеры; повторный запуск выполняется через `up`.

## HTTPS на одном VPS — только по отдельному запросу на развёртывание

Пример ничего не публикует автоматически. Для реального запуска нужны VPS
с Docker, домен с A/AAAA на этот VPS и доступные извне TCP 80/443
(UDP 443 необязателен). Оставьте БД закрытой. Доступ к Docker равнозначен
административному доступу к пользовательским данным.

Создайте приватный `.env.production` на сервере, задайте отдельный пароль БД,
`APP_ENV=production`, `APP_ORIGIN=https://ваш-домен`, `SWEETNET_DOMAIN` без
схемы и пути, `ACME_EMAIL`, `SWEETNET_IMAGE=sweetnet:release-YYYYMMDD`.
`APP_ENV` и `APP_ORIGIN` в файле должны совпадать с production override,
чтобы скрипты обслуживания работали с теми же параметрами. Не копируйте
локальные реальные данные на VPS без отдельного намерения сделать это.

```sh
docker compose --env-file .env.production -f compose.yaml -f deploy/compose.production.yaml -p sweetnet-production config --quiet
docker compose --env-file .env.production -f compose.yaml -f deploy/compose.production.yaml -p sweetnet-production up -d --build --wait --wait-timeout 180
docker compose --env-file .env.production -f compose.yaml -f deploy/compose.production.yaml -p sweetnet-production exec app /app/admin bootstrap --username owner --display-name 'Владелец'
```

Caddy завершает TLS и передаёт запросы `app:8080`, не раздаёт uploads как
статические файлы. Сертификаты сохраняются в собственных volumes. Access log
Caddy выключен. Приложение остаётся привязано к localhost на хосте.
Secure cookie определяется production-настройками backend. Backend не должен
слепо доверять `X-Forwarded-For`; без согласованной настройки доверенного прокси
IP-лимиты могут применяться ко всем участникам за Caddy совместно. Проверка
этого поведения входит в приёмку production.

## Согласованная резервная копия

Перед обслуживанием остановите автоматические обновления и не запускайте
параллельно CLI, изменяющие БД или uploads. Скрипты используют атомарную
блокировку на Docker daemon для одного project; она координирует **эти
скрипты**, но не запрещает ручной `docker compose start` и сторонние записи.
Используйте тот же Docker context и тот же project, что при запуске приложения.

```sh
mkdir -p "$HOME/sweetnet-backups"
chmod 700 "$HOME/sweetnet-backups"
scripts/backup.sh .env sweetnet "$HOME/sweetnet-backups/$(date -u +%Y%m%dT%H%M%SZ)"
docker compose --env-file .env -p sweetnet ps
curl --fail http://localhost:8080/readyz
```

Для production передайте `.env.production sweetnet-production`.
Путь копии должен быть новым каталогом **вне репозитория** с существующим
родителем. Скрипт работает только с поставляемой схемой volumes:
`<project>_postgres_data` и `<project>_uploads`; сторонние override хранения
не поддерживаются.

Скрипт запоминает работающий контейнер, ставит `EXIT/INT/TERM/HUP` trap до
остановки приложения, останавливает приложение и проверяет его состояние.
При работающей БД сохраняет custom-format `pg_dump` и архив uploads,
проверяет архивы, создаёт manifest с SHA-256 и ID образа, запускает исходное
приложение и ждёт readiness до 120 секунд. Helper работает без сети и с отключённым Docker logging, чтобы
байты фотографий не попали в контейнерные логи. Команды не печатают данные
БД, файлы, пароли или содержимое ошибок SQL. Каталог имеет права `0700`,
новые файлы — `0600`. Общая ошибка требует локальной диагностики без
публикации приватных логов.

При ошибке копирования trap пытается запустить приложение. Неполная копия
остаётся с маркером `INCOMPLETE` и не принимается restore. Сбой Docker,
`SIGKILL`, выключение хоста или отказ диска не позволяют гарантировать
выполнение trap: проверьте состояние вручную и выполните `compose start app`.
Не удаляйте контейнер `sweetnet-<project>-maintenance-lock`, пока не проверите,
что предыдущая операция действительно завершилась. Удаляется только этот
служебный контейнер без volumes; пользовательские volumes сохраняются.

Копия включает аккаунты, хеши паролей, приглашения, действующие сессии,
посты и фотографии. Это приватные данные, а SHA-256 не является шифрованием
или доказательством доверенного происхождения. Храните копии зашифрованными
вне VPS; ключ храните отдельно. Не кладите `.env` в этот архив — храните его
отдельно защищённым способом. Доступ к копии предоставляется только владельцу.
Периодичность и срок хранения выберите по допустимой потере данных и объёму.

Сохраните и приложение, использованное при копировании: restore требует
совпадения Docker image ID. Тег может быть изменён после сборки.

```sh
# Выполнять до обновления: save сохраняет именно работающий образ.
docker image save -o "$HOME/sweetnet-backups/app-image.tar" "$(docker inspect --format '{{.Image}}' "$(docker compose --env-file .env -p sweetnet ps -q app)")"
```

Сам образ не содержит `.env` и uploads. Если переносите копию на другой
хост, загрузите сохранённый образ через `docker image load`, присвойте его
ID отдельный тег `docker image tag sha256:ID sweetnet:restore`, укажите этот
тег в файле окружения восстановления. Нужна совместимая архитектура хоста.

## Восстановление только в новое окружение

Исходный project не изменяется и не останавливается. Подготовьте новый
приватный `.env.restore`, новый пароль БД и значения:

```dotenv
APP_ENV=development
APP_ORIGIN=http://localhost:18080
APP_PORT=18080
SWEETNET_IMAGE=sweetnet:restore
```

Добавьте `POSTGRES_USER`, `POSTGRES_DB`, `POSTGRES_PASSWORD` по правилам выше.
Используйте **точный образ** из manifest копии; по умолчанию это предотвращает
неявное восстановление старой БД новым непроверенным приложением.

```sh
chmod 600 .env.restore
scripts/restore.sh .env.restore sweetnet-restore-20260912 "$HOME/sweetnet-backups/ДАТА-КОПИИ" --new-environment
curl --fail http://localhost:18080/readyz
```

Защита требует одновременно флаг `--new-environment`, project, отличный от
источника, отсутствие контейнеров/сети project и отсутствие обоих целевых
volumes. При любой неоднозначности операция прерывается. Никаких `--clean`,
DROP DATABASE или удалений volumes нет. Новый project создаётся только после
проверки manifest, контрольных сумм, ID образа и безопасности путей архива
(запрещены абсолютные пути, `..`, symlink/hardlink и устройства).

Restore запускает только новую БД, импортирует дамп в одной транзакции,
распаковывает uploads без root и без сохранения владельцев, применяет
миграции и запускает приложение с ожиданием readiness. При ошибке импорта
приложение не запускается, данные нового окружения остаются для диагностики.
Повторный restore под этим project запрещён: выберите другое новое имя.
Скрипт не перезапускает частично восстановленный сервис через trap; restart
trap нужен только для ранее работавшего исходного приложения в backup.

После успешного restore выполните реальную приёмку:

1. Войдите владельцем и вторым участником в разных браузерных профилях.
2. Убедитесь, что старые текстовые посты и фотографии доступны обоим.
3. Проверьте гостем приватный API и прямой URL фотографии: доступа нет.
4. Проверьте запрет редактирования чужого поста и доступ отключённого аккаунта.
5. Перезапустите новое окружение и повторите чтение поста и фотографии.

Только эта проверка подтверждает пригодность резервной копии. Изолированный
localhost origin предотвращает использование восстановленных production
cookie браузером на прежнем домене. Не публикуйте проверочное окружение.

## Обновление, откат и сверка файлов

Сначала сделайте и проверьте резервную копию, сохраните текущий образ и
версию кода. Соберите новую версию под новым тегом. На время миграций
остановите приложение — миграции не должны выполняться одновременно со
старой версией сервера:

```sh
docker compose --env-file .env -p sweetnet build
docker compose --env-file .env -p sweetnet stop app
docker compose --env-file .env -p sweetnet run --rm --no-deps migrate /app/admin migrate
# Выполнять только после успешной миграции.
docker compose --env-file .env -p sweetnet up -d --wait --wait-timeout 180 app
```

В production используйте оба `-f` во всех командах. При неудачной миграции
не запускайте сервер автоматически: проверьте совместимость схемы.
Если схема совместима с прежним приложением, верните старый `SWEETNET_IMAGE`
и запустите **без сборки и новых миграций**:

```sh
docker compose --env-file .env -p sweetnet up -d --no-deps --no-build app
```

Не откатывайте схему автоматически. Если старое приложение несовместимо,
восстановите проверенную копию в новом окружении и отдельно спланируйте
переключение; записи после времени копии требуют отдельного решения.

Сверка медиа по умолчанию ничего не удаляет. `--apply` допустим только при
остановленном приложении; пока идёт сверка, не выполняйте другие записи.

```sh
docker compose --env-file .env -p sweetnet stop app
docker compose --env-file .env -p sweetnet run --rm --no-deps migrate /app/admin reconcile-media
# После проверки dry run, при всё ещё остановленном app:
docker compose --env-file .env -p sweetnet run --rm --no-deps migrate /app/admin reconcile-media --apply
docker compose --env-file .env -p sweetnet start app
```

## Доступные проверки tooling

```sh
bash -n scripts/backup.sh scripts/restore.sh deploy/maintenance.sh
python3 -m unittest discover -s deploy/tests -v
docker compose --env-file .env -p sweetnet config --quiet
docker compose --env-file .env.production -f compose.yaml -f deploy/compose.production.yaml -p sweetnet-production config --quiet
git diff --check -- Dockerfile .dockerignore compose.yaml deploy scripts/backup.sh scripts/restore.sh docs/DEPLOYMENT.md
```

Тесты используют временные файлы и подменённую команду Docker для проверки
ветвлений, отказов и trap. Они не подключаются к реальной БД, не изменяют
Docker volumes и **не доказывают** работоспособность контейнерного restore.

Результат 2026-09-12: Bash syntax — успешно; 17 unittest-проверок — успешно,
включая реальный разбор development/production через Docker Compose v5.1.4,
отказ при пустом пароле, ошибки backup с перезапуском, SIGTERM, блокировку
параллельного запуска и защиту restore. Python 3.9.6, Bash 3.2.57.
Проверка пробелов и окончаний строк собственных файлов — успешно.
Контейнерная сборка, runtime, фактические pg_dump/pg_restore с uploads,
права volume после Docker copy-up, браузерная приёмка восстановленной копии
и получение сертификата Caddy остаются непроверенными из-за отсутствия daemon.
На момент проверки tooling ещё отсутствовал `web/package-lock.json`;
перед сборкой владелец frontend должен сформировать и зафиксировать lockfile,
соответствующий `web/package.json`. Dockerfile намеренно требует его для `npm ci`.
