import { computed, ref } from 'vue'

export const ROUTES = [
  { path: '/', name: 'overview', title: 'Обзор' },
  { path: '/quotas', name: 'quotas', title: 'Квоты и расход' },
  { path: '/subscriptions', name: 'subscriptions', title: 'Подписки' },
  { path: '/add', name: 'add', title: 'Добавить аккаунт' },
  { path: '/connect', name: 'connect', title: 'Подключение' },
  { path: '/help', name: 'help', title: 'Справка' },
] as const

export type RouteName = (typeof ROUTES)[number]['name']

function currentPath(): string {
  const raw = window.location.hash.replace(/^#/, '') || '/'
  return raw.split('?')[0] || '/'
}

const path = ref(currentPath())
window.addEventListener('hashchange', () => {
  path.value = currentPath()
})

export const route = computed(() => ROUTES.find((r) => r.path === path.value) ?? ROUTES[0])

export function href(name: RouteName): string {
  const r = ROUTES.find((x) => x.name === name) ?? ROUTES[0]
  return `#${r.path}`
}

export function navigate(name: RouteName): void {
  window.location.hash = href(name)
}
