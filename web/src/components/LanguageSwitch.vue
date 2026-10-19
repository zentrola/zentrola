<script setup lang="ts">
import { computed } from 'vue'
import { useDropdownMenu } from '../dropdownMenu'
import { activeLocale, setLocale, t, type SupportedLocale } from '../i18n'
import Icon from './Icon.vue'

const options: { locale: SupportedLocale; label: string }[] = [
  { locale: 'zh-CN', label: '简体中文' },
  { locale: 'en-US', label: 'English' },
]
const { open, root, trigger, menu, menuId, close, show, onKeydown, onFocusOut } =
  useDropdownMenu('[role="menuitemradio"]')
const currentLabel = computed(
  () => options.find((option) => option.locale === activeLocale.value)?.label ?? '简体中文',
)

function selectLocale(locale: SupportedLocale) {
  setLocale(locale)
  close(true)
}
</script>

<template>
  <div ref="root" class="language-switch" @focusout="onFocusOut">
    <button
      ref="trigger"
      type="button"
      class="language-trigger"
      :aria-label="t('language.label')"
      aria-haspopup="menu"
      :aria-expanded="open"
      :aria-controls="menuId"
      @click="open ? close() : show()"
      @keydown.down.prevent="show()"
      @keydown.up.prevent="show(true)"
      @keydown.esc.prevent="close(true)"
    >
      <Icon class="language-icon" name="website" :size="15" />
      <span>{{ currentLabel }}</span>
      <span class="language-chevron" :class="{ expanded: open }" aria-hidden="true"></span>
    </button>
    <div
      v-if="open"
      :id="menuId"
      ref="menu"
      class="language-dropdown"
      role="menu"
      :aria-label="t('language.label')"
      @keydown="onKeydown"
    >
      <button
        v-for="option in options"
        :key="option.locale"
        type="button"
        role="menuitemradio"
        tabindex="-1"
        class="language-option"
        :aria-checked="activeLocale === option.locale"
        @click="selectLocale(option.locale)"
      >
        <span>{{ option.label }}</span>
        <span class="selected-mark" aria-hidden="true">{{
          activeLocale === option.locale ? '✓' : ''
        }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.language-switch {
  position: relative;
  flex: none;
}
.language-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 112px;
  height: 33px;
  padding: 0 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: #f8fafc;
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 600;
}
.language-trigger:hover,
.language-trigger[aria-expanded='true'] {
  border-color: #cbd5e1;
  background: #fff;
}
.language-trigger:focus-visible {
  outline: 2px solid #bfdbfe;
  outline-offset: 2px;
}
.language-chevron {
  width: 6px;
  height: 6px;
  margin-top: -3px;
  border-right: 1.5px solid var(--color-text-muted);
  border-bottom: 1.5px solid var(--color-text-muted);
  transform: rotate(45deg);
  transition: transform 150ms;
}
.language-chevron.expanded {
  margin-top: 3px;
  transform: rotate(225deg);
}
.language-icon {
  display: none;
}
.language-switch.login-language .language-trigger {
  justify-content: flex-start;
  gap: 8px;
  min-width: 0;
  height: 36px;
  padding: 0 11px;
  border-radius: 9999px;
  background: #fff;
  box-shadow: var(--shadow-card);
}
.language-switch.login-language .language-trigger:hover,
.language-switch.login-language .language-trigger[aria-expanded='true'] {
  background: #f7f9fb;
}
.language-switch.login-language .language-icon {
  display: block;
}
.language-switch.login-language .language-chevron {
  margin-left: 2px;
}
.language-dropdown {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 20;
  width: 140px;
  padding: 5px;
  border: 1px solid var(--line);
  border-radius: var(--radius-control);
  background: #fff;
  box-shadow: 0 12px 28px rgb(15 23 42 / 12%);
}
.language-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 8px 9px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--color-text-secondary);
  text-align: left;
  font-size: 12px;
}
.language-option:hover,
.language-option:focus-visible {
  background: var(--color-primary-soft);
  outline: none;
}
.language-option[aria-checked='true'] {
  color: var(--blue);
  font-weight: 600;
}
.selected-mark {
  width: 14px;
  text-align: center;
}
</style>
