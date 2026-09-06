<script setup lang="ts">
withDefaults(defineProps<{ name: string; size?: number }>(), { size: 20 })
const paths: Record<string, string> = {
  members:
    'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M16 3a4 4 0 0 1 0 8M22 21v-2a4 4 0 0 0-3-3.87M13 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0',
  groups: 'M3 7h7l2 2h9v11H3zM3 7V4h6l2 3',
  models: 'm12 3 9 5-9 5-9-5 9-5Zm-9 9 9 5 9-5M3 16l9 5 9-5',
  resources: 'M8 3v5m8-5v5M5 8h14v3a7 7 0 0 1-14 0V8Zm7 10v4',
  usage: 'M4 3v18h17M8 16v-5m5 5V7m5 9V3',
  operations: 'M6 3h12v18H6zM9 7h6m-6 5h6m-6 5h4',
  plus: 'M12 5v14M5 12h14',
  search: 'M21 21l-5-5M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0',
  close: 'm6 6 12 12M18 6 6 18',
  refresh: 'M20 7a9 9 0 1 0 1 9M20 2v6h-6',
  key: 'M14 6a5 5 0 1 1-4 8l-7 7H1v-4l7-7a5 5 0 0 1 6-4Z',
  eye: 'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Zm13 0a3 3 0 1 1-6 0 3 3 0 0 1 6 0',
  logout: 'M9 4H3v16h6M13 8l4 4-4 4m-5-4h13',
  menu: 'M3 6h18M3 12h18M3 18h18',
  check: 'm5 12 4 4L19 6',
  arrow: 'm9 5 7 7-7 7',
  shield: 'm12 2 9 4v6c0 5-9 10-9 10S3 17 3 12V6l9-4Zm-4 10 3 3 5-6',
}
</script>
<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.7"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="paths[name] || paths.models" />
  </svg>
</template>
