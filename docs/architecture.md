# Архитектура ProxCli

ProxCli управляет CLIProxyAPI через Management API. Весь inference, SSE, WebSocket, планировщик аккаунтов, session affinity и refresh остаются в ядре CLIProxyAPI. Панель не стоит на пути клиентских запросов.

```text
Claude Code / Codex ── Tailscale ── CLIProxyAPI :8317
                                      ├─ auth A → proxy A → provider
                                      └─ auth B → proxy B → provider
Vue :5173 / Go :8787 ── Management API ─┘
         └─ .data/state.enc (только маршруты и OAuth-сессии)
```

Go backend и observer не требуют сторонних библиотек; сборка ядра использует зависимости CLIProxyAPI. Панель Vue не получает management key и OAuth-токены. Для списка моделей Go использует отдельный клиентский access key. API базируется на CLIProxyAPI v8; глобальный proxy читается через совместимый v0 endpoint. Версия upstream при исследовании: commit `6db7d4b`, релиз smoke-теста v8.0.63.

| Операция | Upstream |
| --- | --- |
| Список подписок | GET /v8/management/credentials |
| Изменение proxy/prefix | PATCH /v8/management/credentials/fields |
| Включение/отключение | PATCH /v8/management/credentials/status |
| Начало входа | POST /v8/management/oauth/auth-url |
| Статус входа | GET /v8/management/oauth/status |
| Ручной callback | POST /v8/management/oauth/callback |
| Общий прокси | GET /v0/management/proxy-url |
| Модели | GET /v1/models |

Для PATCH отправляются только имя выбранного auth-файла, `proxy_url` и опциональный prefix. Значения: URL — custom, пустая строка — inherit, `direct` — прямой выход. `null` не используется из-за рассинхронизации upstream metadata/runtime при сбросе этим значением.

Перед PATCH локально сохраняется неподтверждённый маршрут. После успеха upstream он помечается подтверждённым. При падении между шагами UI покажет неизвестное состояние, а не ложное подтверждение. Двухфазная транзакция с upstream невозможна; пользователь может повторить явную запись. Запись файла атомарна через rename, nonce GCM случайный при каждом сохранении.

При OAuth proxy отправляется в POST body до обмена токенов. Маршрут связывается с точным `auth_id` из результата, без эвристики «первый новый файл». Ожидающие сессии переживают перезапуск панели, истекают через 15 минут; upstream и UI могут завершить ожидание раньше. Callback проверяется по state и провайдеру, URL не запрашивается Go-сервером. Вход не переносится автоматически на другой маршрут при ошибке.

API списка не возвращает proxy URL. Поэтому ProxCli хранит только настройки, которыми сам управлял. Это не live-аудит auth-файлов: сторонняя модификация может сделать отображаемый маршрут устаревшим. Для импортированных подписок маршрут неизвестен. Fail-closed на уровне inference потребовал бы изменения ядра; ProxCli этого не обещает.

Подробности наблюдения квот, недеструктивного usage-observer, priority controller и патча `quota-strict` описаны в [quotas-routing.md](quotas-routing.md). Контроллер обновляет только priority и не стоит на пути inference.

Postgres/Redis/RabbitMQ здесь не требуются: один локальный оператор и небольшой объём метаданных. При переходе к многопользовательскому сервису потребуются аутентификация панели, авторизация, независимое хранилище секретов, аудит и блокировки между процессами. Текущую loopback-панель не следует публиковать в интернет.
