<script setup lang="ts">
import { t } from '../i18n'
defineProps<{ name: string }>()
</script>
<template>
  <div class="page-heading">
    <div>
      <h1>{{ t(`nav.${name}`) }}</h1>
      <p>{{ t(`desc.${name}`) }}</p>
    </div>
    <div class="heading-actions"><slot /></div>
  </div>
</template>
