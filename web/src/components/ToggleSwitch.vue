<script setup lang="ts">
// 表单内的布尔开关：底层仍是 checkbox，便于 label 关联与键盘操作。
const checked = defineModel<boolean>({ required: true })
defineProps<{ id?: string; ariaLabel?: string; title?: string; disabled?: boolean }>()
</script>

<template>
  <span class="toggle-switch">
    <input
      :id="id"
      v-model="checked"
      type="checkbox"
      role="switch"
      :aria-label="ariaLabel"
      :title="title"
      :disabled="disabled"
    />
    <span class="toggle-switch-track" aria-hidden="true"></span>
  </span>
</template>

<style scoped>
.toggle-switch {
  position: relative;
  display: inline-flex;
  flex: none;
  flex-direction: row;
  width: 40px;
  height: 22px;
  margin: 0;
  cursor: pointer;
}
.toggle-switch input {
  position: absolute;
  inset: 0;
  z-index: 1;
  width: 100%;
  height: 100%;
  margin: 0;
  opacity: 0;
  cursor: inherit;
}
.toggle-switch-track {
  width: 100%;
  height: 100%;
  border: 1px solid #cbd5e1;
  border-radius: 999px;
  background: #cbd5e1;
  transition:
    border-color 0.15s,
    background 0.15s;
}
.toggle-switch-track::after {
  display: block;
  width: 16px;
  height: 16px;
  margin: 2px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 2px #18324738;
  transition: transform 0.15s;
  content: '';
}
.toggle-switch input:checked + .toggle-switch-track {
  border-color: var(--blue);
  background: var(--blue);
}
.toggle-switch input:checked + .toggle-switch-track::after {
  transform: translateX(18px);
}
.toggle-switch input:focus-visible + .toggle-switch-track {
  outline: 2px solid #bfdbfe;
  outline-offset: 3px;
}
.toggle-switch input:focus-visible {
  outline: 0;
}
.toggle-switch input:disabled + .toggle-switch-track {
  opacity: 0.5;
}
</style>
