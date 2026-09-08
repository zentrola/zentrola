<script setup lang="ts">
import { t } from '../i18n'
withDefaults(defineProps<{ name: string; showDescription?: boolean }>(), {
  showDescription: true,
})
</script>
<template>
  <div class="page-heading">
    <div>
      <h1>{{ t(`nav.${name}`) }}</h1>
      <p v-if="showDescription">{{ t(`desc.${name}`) }}</p>
    </div>
    <div class="heading-actions"><slot /></div>
  </div>
</template>
