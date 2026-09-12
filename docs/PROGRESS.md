# Прогресс SweetNet

Начало реализации: 2026-09-12. Исходный коммит: b39b5a4.

| Этап | Статус | Проверки / следующий шаг |
| --- | --- | --- |
| 0 — каркас | Выполнен | Go `go vet`, frontend build и OpenAPI validation пройдены; Docker daemon недоступен |
| 1 — PostgreSQL | Выполнен | Миграции повторяемы; backend integration tests пройдены на отдельной PostgreSQL 17.11 |
| 2 — аккаунты/API | Выполнен | Приглашения, сессии, Origin, роли, отзыв сессий и права покрыты integration tests |
| 3 — посты/фото | Выполнен | CRUD, курсорная лента, валидация изображений и защищённая media выдача покрыты tests |
| 4 — интерфейс | Выполнен | React подключён к API; полный Playwright-сценарий двух browser contexts пройден на desktop и 360 px |
| 5 — интеграция | Выполнен | 2 Playwright-теста пройдены; backend integration tests, frontend checks и OpenAPI validation пройдены |
| 6 — эксплуатация | Не выполнен | Инструкция, Docker, backup/restore |

## Git для ревью

Последовательные ветки: `codex/01-foundation` → `codex/02-backend-api` → `codex/03-web-app` → `codex/04-release`.
Каждая следующая включает предыдущую. `main` остаётся исходной точкой. Коммиты разбиваются по функциональным слоям; ветки не публикуются автоматически.

## Следующий шаг

Проверить Docker Compose, backup/restore и новый checkout при доступном Docker daemon; production HTTPS остаётся непроверенным.
