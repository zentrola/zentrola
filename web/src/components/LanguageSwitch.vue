<script setup lang="ts">
import { activeLocale, setLocale, t, type SupportedLocale } from '../i18n'

const options: { locale: SupportedLocale; label: string }[] = [
  { locale: 'zh-CN', label: '中' },
  { locale: 'en-US', label: 'EN' },
]
</script>

<template>
  <div class="language-switch" role="group" :aria-label="t('language.label')">
    <button
      v-for="option in options"
      :key="option.locale"
      type="button"
      :aria-pressed="activeLocale === option.locale"
      :title="t(option.locale === 'zh-CN' ? 'language.chinese' : 'language.english')"
      @click="setLocale(option.locale)"
    >
      {{ option.label }}
    </button>
  </div>
</template>

<style scoped>
.language-switch {
  display: inline-flex;
  flex: none;
  padding: 2px;
  border: 1px solid #d9e2ec;
  border-radius: 7px;
  background: #f4f7fa;
}
.language-switch button {
  min-width: 31px;
  height: 27px;
  padding: 0 7px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: #788a9b;
  font-size: 11px;
  font-weight: 600;
}
.language-switch button:hover:not([aria-pressed='true']) {
  color: #405a72;
}
.language-switch button[aria-pressed='true'] {
  background: #fff;
  color: var(--blue);
  box-shadow: 0 1px 3px #172f4617;
}
.language-switch button:focus-visible {
  outline: 2px solid #90b7fb;
  outline-offset: 2px;
}
</style>
