<template>
  <svg class="mark" viewBox="0 0 32 32" aria-hidden="true" focusable="false">
    <rect x="0.5" y="0.5" width="31" height="31" rx="8" class="mark__tile" />
    <path d="M9 11h5c3.6 0 3.6 10 7.2 10H23" fill="none" class="mark__wire" stroke-width="2.3" stroke-linecap="round" />
    <circle cx="8.5" cy="11" r="3" class="mark__from" />
    <circle cx="23.5" cy="21" r="3" class="mark__to" />
  </svg>
</template>

<style scoped>
.mark {
  width: 28px;
  height: 28px;
  flex: none;
}
.mark__tile {
  fill: #1c1c1c;
  stroke: #2c2c2c;
}
.mark__wire {
  stroke: #fafafa;
}
.mark__from {
  fill: #8a8a8a;
}
.mark__to {
  fill: #fafafa;
}
</style>
