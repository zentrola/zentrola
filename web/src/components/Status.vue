<script setup lang="ts">
import { codeText } from '../i18n'
defineProps<{ value: string }>()
</script>
<template>
  <span class="status" :class="value.toLowerCase()"><i></i>{{ codeText('state', value) }}</span>
</template>
