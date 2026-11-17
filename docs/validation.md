# Проверка

## Обновление CLIProxyAPI до v8.0.76

Проверено 2026-11-18 на macOS arm64, Go 1.27.4 и Bun 1.4.5-canary.1. Закреплён upstream commit `7df56074b252384d878ec4fca840552c6fb5236e`.

- Сборка из чистого upstream: проверка SHA, применение патча, полные auth/session suites, SDK routing/state tests, native loader tests, сборка ядра и observer — успешно.
- `bun run check`: Go race tests, `go vet`, frontend tests, typecheck и production build — успешно. Новые тесты обнаружения upstream-релизов отдельно проверяют числовой порядок версий, нестабильные/некорректные теги и ошибки GitHub API — успешно.
- `TestProxCliQuotaStrict` расширен на single/mixed selection для explicit и LCP affinity. Дополнительный тест проверяет квоту, исчерпанную другой сессией во время вызова scheduler, и последующий пересчёт после исключения кандидата — успешно.
- Native observer C ABI и contract test с настоящим v8.0.76, включённой политикой и observer (панель с race detector) — успешно. Использованы синтетические credentials и отдельные каталоги.
- `actionlint` v1.7.12 проверил workflows — успешно. Реальный запрос GitHub API определил v8.0.76 как текущий стабильный релиз.

`Check` теперь проверяет закреплённое ядро, observer и контракт панели на PR/push. `Upstream compatibility` проверяет новые стабильные релизы ежедневно и вручную на Linux amd64. Его результаты появятся после запуска в GitHub Actions; локально Linux/Windows/Intel macOS не проверялись. Автоматической смены pin и публикации нет. Платформенную матрицу сохраняет release workflow.

## Предыдущая проверка v8.0.63

Локальная проверка выполнена 2026-11-10 на macOS arm64, Go 1.27.3 и Bun 1.4.4-canary.1.

## Автоматические проверки

- `bun run check`: Go tests с race detector, `go vet`, 8 frontend tests, Vue typecheck и Vite production build — успешно.
- `bun run build:core`: точный upstream `6db7d4b839d86c5b24e17bbe73bcd4db18ef1b1a`, проверка применимости патча, полный auth test suite, SDK routing config test, native loader tests, сборка core и observer — успешно.
- `TestProxCliQuotaStrict` внутри патча: explicit и LCP, подтверждённая квота, 5xx, сеть, 401/403, обычный rate limit, сохранение нового binding после восстановления более приоритетного аккаунта.
- `scripts/test-plugin.py`: реальная C ABI регистрация, init→shutdown без register, register→usage→shutdown→register→usage, запись cache counters и отсутствие исходных секретов — успешно.
- Тесты нормализации отдельно проверяют, что Claude cache creation не считается чтением и что у Codex cached subset вычитается из input.
- Реальный CLIProxyAPI contract test: список auth, per-auth proxy/prefix persistence, toggle, direct/inherit, OAuth URL с custom proxy, strict capability, применение priority с сохранением других auth-полей и загрузка observer — успешно.
- Распакованный macOS arm64 архив: `version`, генерация конфигурации через `init` и HTTP-запуск UI без Go/Bun — успешно. Тестовые ключи и каталог удалены после проверки.
- Публичный набор исходников проверен Gitleaks; релизный архив проверен на отсутствие `.env`, `.data`, auth-файлов и `node_modules`.
- `actionlint` для workflow — успешно. Платформенные build/test/package выполняются самим release workflow; результаты доступны в GitHub Actions.

Запуск contract test (путь и расширение подставляются под ОС):

```sh
PROXCLI_TEST_CORE="$PWD/bin/cli-proxy-api" \
PROXCLI_TEST_PLUGIN="$PWD/bin/plugins/proxcli-usage.dylib" \
PROXCLI_TEST_POLICY=true \
go test ./internal/hub -run TestCLIProxyAPIIntegration -count=1 -v
```

- Исправление Intel macOS воспроизведено на минимальном Go→C ABI хосте. После изоляции C-потоков настоящий x86-64 CLIProxyAPI + observer прошёл contract test локально под Rosetta с race detector у панели. Native runner macOS Intel проверяется перед публикацией релиза.

## Проверка UI

Панель открыта в браузере с демо API, без личных аккаунтов. Проверены графики квот/расхода, список подписок и раскрытый proxy editor. Сохранённые PNG находятся в `docs/images`. Для desktop использована ширина 1600 CSS px, для мобильной проверки — 390 px. Исправлены intrinsic grid и segmented controls, вызывавшие горизонтальный overflow.

Демо API обслуживает только тестовые значения, не читает токены и не выполняет inference/OAuth. Значения на скриншотах не являются измерениями личных подписок.

## Границы проверки

Реальные OAuth code exchange, refresh, inference, фактический IP прокси и provider-specific ограничения требуют собственных аккаунтов и рабочего прокси. Эти действия с личными аккаунтами не выполнялись. Contract test использует синтетический auth-файл с недействительным токеном и отдельный временный каталог; OAuth URL только создаётся, но не открывается.

Для новой локальной сборки восстановление привязок проверяется отдельными процессами, см. ниже. Старый релиз v0.2.0 сохраняет affinity только в памяти. Observer — best-effort, а не биллинговый журнал exactly-once.

На Intel macOS loader-проверка использует `TestProxCliUsageNativeThreadLifecycle`: init/call/free/shutdown, замена библиотеки и повторное открытие того же образа через изолированный путь `proxcli-usage`. Штатные тесты с другими Go-плагинами вызывают известный конфликт Go runtime на этой платформе и не входят в этот target. На остальных платформах запускается весь набор Unix/Windows loader tests. Contract test с настоящим ядром и observer обязателен на каждой платформе.

## Долговечные привязки

Тест `TestProxCliDurableProcessRestart` запускает отдельные процессы без вызова Stop. После восстановления при другом fallback сохраняется прежний аккаунт для явной и выросшей LCP-сессии. Дополнительно проверяются TTL с учётом простоя, сокращение TTL, удаление аккаунта, повреждение/исчезновение снимка, потерянный/неверный ключ и отказ записи. Политика quota-strict проверяется после восстановления для квоты, 5xx, сети, 401/403 и обычного rate limit. Файлы зашифрованы, на POSIX проверены права 0600. Это не проверка реального cache hit у провайдера.
