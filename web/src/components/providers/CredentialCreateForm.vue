<script setup lang="ts">
import { computed, ref } from 'vue'
import { t } from '../../i18n'
import {
  validApiKeyCredential,
  validSubscriptionCredential,
  type CredentialAuthType,
  type CredentialInput,
} from '../../providerCredentials'
import { showErrorToast } from '../../toast'
import type { Provider } from '../../types'
import SubscriptionImport, { type SubscriptionInputMode } from './SubscriptionImport.vue'

// 表单由外层弹窗的页脚按钮通过 form 属性提交。
const props = defineProps<{
  provider: Provider
  busy: boolean
  // 保存成功返回 true。
  createCredential: (input: CredentialInput) => Promise<boolean>
}>()
const emit = defineEmits<{ saved: [] }>()

const authType = ref<CredentialAuthType>('API_KEY')
const inputMode = ref<SubscriptionInputMode>('UPLOAD')
const fileName = ref('')
const credential = ref('')
const subscriptionAdapter = computed(
  () => props.provider.authAdapters?.find((adapter) => adapter !== 'API_KEY') ?? '',
)
const claudeSubscription = computed(() => subscriptionAdapter.value === 'ANTHROPIC_CLAUDE_CODE')
const subscriptionRequiredText = computed(() =>
  t(
    claudeSubscription.value
      ? 'resources.claudeSubscriptionRequired'
      : 'resources.subscriptionRequired',
  ),
)

function resetInput() {
  inputMode.value = 'UPLOAD'
  fileName.value = ''
  credential.value = ''
}

async function save() {
  const valid =
    authType.value === 'API_KEY'
      ? validApiKeyCredential(credential.value)
      : validSubscriptionCredential(credential.value, claudeSubscription.value)
  if (!valid) {
    showErrorToast(
      authType.value === 'API_KEY'
        ? t('resources.credentialRequired')
        : subscriptionRequiredText.value,
    )
    return
  }
  const saved = await props.createCredential({
    providerId: props.provider.id,
    name: `${props.provider.name} ${t(`resources.authTypes.${authType.value}`)}`,
    credential: credential.value,
    authType: authType.value,
    authAdapter: authType.value === 'SUBSCRIPTION' ? subscriptionAdapter.value : 'API_KEY',
  })
  if (saved) {
    resetInput()
    emit('saved')
  }
}
</script>

<template>
  <form id="credential-create-form" class="credential-create-form" @submit.prevent="save">
    <p class="muted">{{ t('resources.createHint') }}</p>
    <div class="credential-form-row">
      <label class="credential-form-label" for="credential-auth-type">{{
        t('resources.authType')
      }}</label>
      <div class="credential-form-control">
        <select id="credential-auth-type" v-model="authType" :disabled="busy" @change="resetInput">
          <option value="API_KEY">{{ t('resources.authTypes.API_KEY') }}</option>
          <option
            v-if="provider.authAdapters?.some((adapter) => adapter !== 'API_KEY')"
            value="SUBSCRIPTION"
          >
            {{ t('resources.authTypes.SUBSCRIPTION') }}
          </option>
        </select>
      </div>
    </div>
    <div v-if="authType === 'API_KEY'" class="credential-form-row">
      <label class="credential-form-label" for="credential-api-key">{{
        t('resources.apiKey')
      }}</label>
      <div class="credential-form-control">
        <input
          id="credential-api-key"
          v-model="credential"
          type="text"
          autocomplete="off"
          required
          autofocus
          :disabled="busy"
          spellcheck="false"
        />
      </div>
    </div>
    <p v-if="authType === 'API_KEY'" class="field-hint credential-form-hint">
      {{ t('resources.credentialHint') }}
    </p>
    <SubscriptionImport
      v-else
      v-model:mode="inputMode"
      v-model:credential="credential"
      v-model:file-name="fileName"
      :claude-subscription="claudeSubscription"
      :busy="busy"
      :invalid-text="subscriptionRequiredText"
    />
  </form>
</template>

<style scoped>
.credential-create-form {
  display: grid;
  gap: 14px;
}
.credential-create-form > .muted {
  margin: 0;
  line-height: 1.6;
}
.credential-form-row {
  display: grid;
  grid-template-columns: 86px minmax(0, 1fr);
  gap: 14px;
  align-items: start;
}
.credential-form-label {
  display: block;
  margin: 0;
  padding-top: 10px;
  color: var(--color-text-secondary);
  line-height: 1.4;
  text-align: right;
}
.credential-form-control {
  min-width: 0;
}
.credential-form-hint {
  margin: -2px 0 0 100px;
}
.subscription-import {
  display: grid;
  min-width: 0;
  gap: 12px;
}
@media (max-width: 760px) {
  .credential-form-row {
    grid-template-columns: 1fr;
    gap: 6px;
  }
  .credential-form-label {
    padding-top: 0;
    text-align: left;
  }
  .credential-form-hint {
    margin-left: 0;
  }
}
</style>
