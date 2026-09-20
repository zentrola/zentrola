<script setup lang="ts">
withDefaults(defineProps<{ name: string; size?: number }>(), { size: 20 })
const paths: Record<string, string> = {
  home: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z',
  members:
    'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M16 3a4 4 0 0 1 0 8M22 21v-2a4 4 0 0 0-3-3.87M13 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0',
  groups: 'M3 7h7l2 2h9v11H3zM3 7V4h6l2 3',
  models: 'm12 3 9 5-9 5-9-5 9-5Zm-9 9 9 5 9-5M3 16l9 5 9-5',
  providers: 'M4 7h16v12H4zM8 7V4h8v3M8 12h8m-8 4h5',
  connection: 'M8 3v5m8-5v5M5 8h14v3a7 7 0 0 1-14 0V8Zm7 10v4',
  guide: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20Zm3.8-13.8-2.2 5.4-5.4 2.2 2.2-5.4 5.4-2.2Z',
  activity: 'M3 12h4l2-6 4 12 3-9 2 3h3',
  external: 'M14 4h6v6M20 4l-9 9M18 13v6H5V6h6',
  website:
    'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18ZM3 12h18M12 3c2.2 2.5 3.3 5.5 3.3 9S14.2 18.5 12 21M12 3C9.8 5.5 8.7 8.5 8.7 12s1.1 6.5 3.3 9',
  edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L8 18l-4 1 1-4L16.5 3.5Z',
  resources: 'M8 3v5m8-5v5M5 8h14v3a7 7 0 0 1-14 0V8Zm7 10v4',
  usage: 'M4 3v18h17M8 16v-5m5 5V7m5 9V3',
  operations: 'M3 12a9 9 0 1 0 3-6.7L3 8M3 3v5h5M12 7v5l3 2',
  plus: 'M12 5v14M5 12h14',
  upload: 'M12 16V4m0 0-5 5m5-5 5 5M4 16v4h16v-4',
  calendar: 'M7 3v3m10-3v3M4 9h16M5 5h14a1 1 0 0 1 1 1v14H4V6a1 1 0 0 1 1-1Z',
  close: 'm6 6 12 12M18 6 6 18',
  trash: 'M3 6h18M8 6V4h8v2m-10 0 1 15h10l1-15M10 11v6m4-6v6',
  refresh: 'M20 7a9 9 0 1 0 1 9M20 2v6h-6',
  copy: 'M9 8h10v12H9zM5 16H4V4h10v1',
  key: 'M11 11a4 4 0 1 1-8 0 4 4 0 0 1 8 0Zm0 0h11M18 11v3M21 11v2',
  lock: 'M7 10V7a5 5 0 0 1 10 0v3M6 10h12a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2Zm6 4v3',
  'lock-open':
    'M7 10V7a5 5 0 0 1 9.8-1.4M6 10h12a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2Zm6 4v3',
  eye: 'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Zm13 0a3 3 0 1 1-6 0 3 3 0 0 1 6 0',
  logout: 'M9 4H3v16h6M13 8l4 4-4 4m-5-4h13',
  menu: 'M3 6h18M3 12h18M3 18h18',
  check: 'm5 12 4 4L19 6',
  alert: 'M12 3 2.8 20h18.4L12 3Zm0 6v4m0 3.5v.5',
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
