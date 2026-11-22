<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'

const form = ref({
  provider: '',
  model: '',
  base_url: '',
  api_key: '',
  max_steps: 40,
  max_tokens: 20000,
  max_duration_seconds: 600,
  ego_enabled: false,
  laya_enabled: false,
  jev_enabled: false,
})
const apiKeyConfigured = ref(false)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')

function applySettings(settings) {
  form.value = {
    ...form.value,
    provider: settings.provider || '',
    model: settings.model || '',
    base_url: settings.base_url || '',
    api_key: '',
    max_steps: settings.max_steps,
    max_tokens: settings.max_tokens,
    max_duration_seconds: settings.max_duration_seconds,
    ego_enabled: settings.ego_enabled,
    laya_enabled: settings.laya_enabled,
    jev_enabled: settings.jev_enabled,
  }
  apiKeyConfigured.value = settings.api_key_configured
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    applySettings(await api.get('/api/settings'))
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function save() {
  error.value = ''
  notice.value = ''
  saving.value = true
  try {
    const saved = await api.put('/api/settings', form.value)
    applySettings(saved)
    notice.value = '设置已保存'
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="state" role="status">正在读取设置…</div>
  <div v-else class="settings-page">
    <p v-if="error" class="message error" role="alert">{{ error }}</p>
    <p v-if="notice" class="message success" role="status">{{ notice }}</p>

    <form class="settings-form" @submit.prevent="save">
      <section class="section" aria-labelledby="model-settings-title">
        <header class="section-head">
          <h3 id="model-settings-title">模型连接</h3>
          <span v-if="apiKeyConfigured" class="configured">API Key 已配置</span>
          <span v-else class="muted">API Key 未配置</span>
        </header>

        <div class="form-grid">
          <label class="field">
            <span>模型服务商</span>
            <input v-model="form.provider" name="provider" required autocomplete="off" />
          </label>
          <label class="field">
            <span>模型名称</span>
            <input v-model="form.model" name="model" required autocomplete="off" />
          </label>
          <label class="field wide">
            <span>API Base URL</span>
            <input v-model="form.base_url" name="base_url" type="url" required autocapitalize="none" spellcheck="false" />
          </label>
          <label class="field wide">
            <span>API Key</span>
            <input
              v-model="form.api_key"
              name="api_key"
              type="password"
              autocomplete="new-password"
              placeholder="仅在需要新增或更换时填写"
              :aria-describedby="apiKeyConfigured ? 'api-key-status' : undefined"
            />
            <small v-if="apiKeyConfigured" id="api-key-status">留空会保留当前密钥。</small>
          </label>
        </div>
      </section>

      <section class="section" aria-labelledby="budget-settings-title">
        <header class="section-head">
          <h3 id="budget-settings-title">运行预算</h3>
        </header>
        <div class="form-grid budget-grid">
          <label class="field">
            <span>最大步骤数</span>
            <input v-model.number="form.max_steps" name="max_steps" type="number" min="1" required />
          </label>
          <label class="field">
            <span>最大 Token 数</span>
            <input v-model.number="form.max_tokens" name="max_tokens" type="number" min="1" required />
          </label>
          <label class="field">
            <span>最长运行时间（秒）</span>
            <input v-model.number="form.max_duration_seconds" name="max_duration_seconds" type="number" min="1" required />
          </label>
        </div>
      </section>

      <section class="section" aria-labelledby="enhanced-settings-title">
        <header class="section-head">
          <h3 id="enhanced-settings-title">增强档</h3>
        </header>
        <div class="switch-list">
          <label class="switch-row">
            <input v-model="form.ego_enabled" name="ego_enabled" type="checkbox" />
            <span>Ego 浏览器</span>
          </label>
          <label class="switch-row">
            <input v-model="form.laya_enabled" name="laya_enabled" type="checkbox" />
            <span>Laya 决策</span>
          </label>
          <label class="switch-row">
            <input v-model="form.jev_enabled" name="jev_enabled" type="checkbox" />
            <span>JEV 决策</span>
          </label>
        </div>
      </section>

      <footer class="form-actions">
        <button class="primary" type="submit" :disabled="saving">{{ saving ? '保存中…' : '保存设置' }}</button>
      </footer>
    </form>
  </div>
</template>

<style scoped>
.settings-page { max-width: 820px; }
.settings-form { display: flex; flex-direction: column; gap: 14px; }
.section { padding: 18px 20px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); }
.section-head { display: flex; align-items: center; gap: 12px; margin-bottom: 18px; }
.section-head h3 { margin: 0; font-size: 14px; font-weight: 600; }
.configured, .muted { color: var(--muted); font-size: 12px; }
.configured { color: var(--ok); }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.wide { grid-column: 1 / -1; }
.field { display: flex; flex-direction: column; gap: 5px; min-width: 0; }
.field > span { font-size: 12px; color: var(--muted); }
.field input { width: 100%; min-height: 36px; }
.field input:focus-visible, .switch-row input:focus-visible, button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.field small { color: var(--muted); font-size: 12px; }
.budget-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.switch-list { display: flex; flex-wrap: wrap; gap: 12px 24px; }
.switch-row { display: inline-flex; align-items: center; gap: 8px; min-height: 32px; }
.switch-row input { accent-color: var(--accent); width: 16px; height: 16px; }
.form-actions { display: flex; justify-content: flex-end; }
.form-actions button { min-height: 36px; }
.form-actions button:disabled { opacity: .55; cursor: wait; }
.message, .state { margin: 0 0 12px; font-size: 13px; }
.error { color: var(--bad); }
.success { color: var(--ok); }
@media (max-width: 640px) {
  .form-grid, .budget-grid { grid-template-columns: 1fr; }
  .wide { grid-column: auto; }
  .section { padding: 16px; }
}
</style>
