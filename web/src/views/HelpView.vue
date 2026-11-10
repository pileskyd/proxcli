<script setup lang="ts">
const DOCS = [
  {
    href: 'https://gist.github.com/t3dotgg/59a521d2ee8d70aaf90e280c175072af',
    title: 'Гайд Theo по хабу CLIProxyAPI',
    text: 'Установка, config.yaml, Tailscale и настройка клиентов. Эта панель построена по нему.',
  },
  {
    href: 'https://github.com/router-for-me/CLIProxyAPI',
    title: 'CLIProxyAPI на GitHub',
    text: 'Исходный код, релизы и описание файлов аккаунтов.',
  },
  {
    href: 'https://help.router-for.me/',
    title: 'Документация CLIProxyAPI',
    text: 'Все параметры config.yaml, маршрутизация и API управления.',
  },
  {
    href: 'https://code.claude.com/docs/en/settings',
    title: 'Настройки Claude Code',
    text: 'Где лежит settings.json и как работают переменные env.',
  },
  {
    href: 'https://learn.chatgpt.com/docs/config-file/config-advanced',
    title: 'Настройки Codex',
    text: 'Формат config.toml и описание model_providers.',
  },
  {
    href: 'https://tailscale.com/kb/1312/serve',
    title: 'Tailscale Serve',
    text: 'Как опубликовать локальный порт хаба внутри tailnet.',
  },
]
const docs = DOCS.map((d) => ({ ...d, host: new URL(d.href).host }))

const policy = [
  {
    q: 'Куда идёт новая сессия',
    a: 'На аккаунт провайдера с ближайшим сбросом известного общего окна квоты. Свежий положительный остаток важнее неизвестного, исчерпанные и отключённые аккаунты — в конце. ProxCli меняет только поле priority; выбор делает встроенный scheduler CLIProxyAPI (fill-first + session affinity), а не scheduler-плагин.',
  },
  {
    q: 'Когда активная сессия меняет аккаунт',
    a: 'Только при подтверждённом исчерпании квоты этого аккаунта (режим quota-strict). Ошибки сети, 5xx, 401/403, обычный 429 и смена приоритетов сессию не переносят: запрос завершится ошибкой или повторится на том же аккаунте.',
  },
  {
    q: 'Сколько живёт привязка сессии',
    a: 'TTL по умолчанию — 168 часов простоя, каждое обращение продлевает его. Обновлённое ядро при запуске через ProxCli сохраняет привязки на диск и восстанавливает после перезапуска. Сохраняйте affinity.enc вместе с affinity.enc.key. Кеш остаётся у провайдера и может истечь по своему TTL.',
  },
  {
    q: 'Откуда проценты квоты',
    a: 'Из usage API провайдера (не чаще раза в 5 минут на аккаунт) и из заголовков ответов. Наблюдение старше 10 минут не влияет на приоритет. Прошедший сброс не означает 100%: нужно новое наблюдение. Проценты разных окон не складываются и не пересчитываются в токены.',
  },
  {
    q: 'Насколько точна история расхода',
    a: 'Журнал хранится 30 дней. Его пишет плагин proxcli-usage по принципу best-effort: при переполнении очереди или ошибке диска счётчики «потеряно» и «ошибок записи» на экране квот растут, а история за это время неполная. Это не биллинговый учёт.',
  },
  {
    q: 'Что нужно для работы',
    a: 'Собранное ядро ProxCli (bun run build:core, затем bun run hub) и в его конфиге routing.strategy=fill-first, session-affinity=true, session-affinity-rebind-on=quota-strict и включённый плагин proxcli-usage. Подробно — в файле docs/quotas-routing.md репозитория.',
  },
]

const problems = [
  {
    q: 'Codex отвечает 401 token_invalidated',
    a: 'Токен аккаунта отозван. Войдите заново: в разделе «Добавить аккаунт» или командой ./cli-proxy-api -codex-device-login на хабе.',
  },
  {
    q: 'Лимит кончился, а запросы не переключаются',
    a: 'Держите хотя бы две включённые подписки каждого провайдера, иначе хабу не на что переключиться. Активная сессия переносится только после подтверждённого исчерпания; на экране «Квоты и расход» видно, применяется ли политика.',
  },
  {
    q: 'Нужно временно убрать аккаунт из ротации',
    a: 'Отключите подписку переключателем в списке. Перезапуск хаба не нужен, вход сохраняется.',
  },
  {
    q: 'macOS не находит адреса *.ts.net',
    a: 'С открытым tailscaled создайте файл /etc/resolver/<tailnet>.ts.net со строкой nameserver 100.100.100.100.',
  },
  {
    q: 'В счётчиках кеша почти нет чтений',
    a: 'Проверьте, что в routing включена session-affinity: тогда разговор держится одного аккаунта и кеш срабатывает.',
  },
]
</script>

<template>
  <div class="page">
    <header class="page__head">
      <div>
        <h1 tabindex="-1">Справка</h1>
        <p class="page__lede">
          Использование чужих подписок через общий хаб может нарушать условия провайдеров. Аккаунты могут заблокировать, решение за вами.
        </p>
      </div>
    </header>

    <section class="panel block" aria-labelledby="docs-title">
      <h2 id="docs-title" class="panel__title">Документация</h2>
      <ul class="docs">
        <li v-for="d in docs" :key="d.href" class="doc">
          <a :href="d.href" target="_blank" rel="noopener noreferrer" class="doc__link">
            {{ d.title }}<span class="visually-hidden"> (откроется в новой вкладке)</span>
          </a>
          <span class="doc__host mono">{{ d.host }}</span>
          <p class="doc__text muted">{{ d.text }}</p>
        </li>
      </ul>
    </section>

    <section class="panel block" aria-labelledby="policy-title">
      <h2 id="policy-title" class="panel__title">Квоты и выбор аккаунта</h2>
      <dl class="faq">
        <template v-for="p in policy" :key="p.q">
          <dt>{{ p.q }}</dt>
          <dd>{{ p.a }}</dd>
        </template>
      </dl>
    </section>

    <section class="panel block" aria-labelledby="faq-title">
      <h2 id="faq-title" class="panel__title">Если что-то пошло не так</h2>
      <dl class="faq">
        <template v-for="p in problems" :key="p.q">
          <dt>{{ p.q }}</dt>
          <dd>{{ p.a }}</dd>
        </template>
      </dl>
    </section>
  </div>
</template>

<style scoped>
.block {
  display: grid;
  gap: var(--space-3);
  padding: var(--space-5) var(--space-5) var(--space-3);
}
.docs {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 var(--space-6);
  margin: 0;
  padding: 0;
  list-style: none;
}
.doc {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: baseline;
  gap: 2px var(--space-3);
  padding: var(--space-3) 0;
  border-top: 1px solid var(--rule);
}
.doc__link {
  font-weight: 500;
  border-radius: 4px;
}
.doc__host {
  font-size: var(--fs-xs);
  color: var(--ink-3);
}
.doc__text {
  grid-column: 1 / -1;
  font-size: var(--fs-sm);
}

.faq {
  display: grid;
  grid-template-columns: minmax(0, 18rem) minmax(0, 1fr);
  margin: 0;
  max-width: 980px;
}
.faq dt,
.faq dd {
  margin: 0;
  padding: var(--space-3) 0;
  border-top: 1px solid var(--rule);
}
.faq dt {
  padding-right: var(--space-5);
  font-weight: 500;
}
.faq dd {
  color: var(--ink-2);
  max-width: var(--measure);
}

@media (max-width: 760px) {
  .block {
    padding: var(--space-4) var(--space-4) var(--space-2);
  }
  .docs {
    grid-template-columns: 1fr;
  }
  .faq {
    grid-template-columns: 1fr;
  }
  .faq dd {
    border-top: 0;
    padding-top: 0;
  }
}
</style>
