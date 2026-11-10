<template>
  <p class="legend" aria-hidden="true">
    <span class="legend__title">Маршрут</span>
    <span class="legend__item legend__item--custom">свой прокси</span>
    <span class="legend__item legend__item--default">общий прокси</span>
    <span class="legend__item legend__item--direct">без прокси, IP хаба</span>
    <span class="legend__item legend__item--unknown">неизвестен</span>
  </p>
</template>

<style scoped>
.legend {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-5);
  max-width: none;
  padding: 0 var(--space-5);
  color: var(--ink-2);
  font-size: var(--fs-sm);
}
.legend__title {
  color: var(--ink);
}
.legend__item {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}
.legend__item::before {
  content: '';
  width: 18px;
  border-top: 2px solid var(--c);
}
.legend__item--custom {
  --c: var(--route-custom);
}
.legend__item--default {
  --c: var(--route-default);
}
.legend__item--default::before {
  border-top-style: dashed;
}
.legend__item--direct {
  --c: var(--route-direct);
}
.legend__item--direct::before {
  border-top-style: dotted;
  border-top-width: 3px;
}
.legend__item--unknown {
  --c: var(--ink-3);
}
.legend__item--unknown::before {
  border-top-style: dashed;
  border-top-width: 1px;
}

@media (max-width: 900px) {
  .legend {
    padding: 0;
  }
}
</style>
