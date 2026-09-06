<script setup lang="ts">
import { t, i18n } from '../i18n'
defineProps<{ value: string }>()
</script>
<template>
  <span class="status" :class="value.toLowerCase()"
    ><i></i>{{ i18n.global.te(`state.${value}`) ? t(`state.${value}`) : value }}</span
  >
</template>
