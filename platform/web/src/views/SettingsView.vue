<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import DesktopWorkerActivation from '../components/DesktopWorkerActivation.vue'

const form = ref({
  provider: '',
  model: '',
  base_url: '',
  api_key: '',
  max_steps: 40,
  max_tokens: 20000,
  max_duration_seconds: 600,
  playwright_browser: '',
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
    playwright_browser: settings.playwright_browser || '',
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
    <DesktopWorkerActivation />
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
            <input v-model="form.model" name="model" :required="Boolean(form.base_url || form.api_key)" autocomplete="off" />
          </label>
          <label class="field wide">
            <span>API Base URL</span>
            <input v-model="form.base_url" name="base_url" type="url" :required="Boolean(form.model || form.api_key)" autocapitalize="none" spellcheck="false" />
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

      <section class="section" aria-labelledby="browser-settings-title">
        <header class="section-head">
          <h3 id="browser-settings-title">浏览器</h3>
        </header>
        <div class="form-grid">
          <div class="field">
            <label for="playwright-browser">Playwright 浏览器</label>
            <select id="playwright-browser" v-model="form.playwright_browser" name="playwright_browser" aria-describedby="playwright-browser-status">
              <option value="">执行机默认</option>
              <option value="chromium">Chromium</option>
              <option value="firefox">Firefox</option>
              <option value="webkit">WebKit</option>
              <option value="chrome">Google Chrome</option>
              <option value="msedge">Microsoft Edge</option>
            </select>
            <small id="playwright-browser-status">执行机需安装所选浏览器，缺失时运行会报错。</small>
          </div>
        </div>
      </section>

      <section class="section" aria-labelledby="enhanced-settings-title">
        <header class="section-head">
          <h3 id="enhanced-settings-title">增强档</h3>
        </header>
        <div class="switch-list">
          <div class="field">
            <label class="switch-row">
              <input v-model="form.ego_enabled" name="ego_enabled" type="checkbox" aria-describedby="ego-provider-status" />
              <span>请求 Ego 浏览器</span>
            </label>
            <small id="ego-provider-status">仅支持本机冒烟。租户 UI 运行记录降级，使用 Playwright。</small>
          </div>
          <div class="field">
            <label class="switch-row">
              <input v-model="form.laya_enabled" name="laya_enabled" type="checkbox" aria-describedby="laya-provider-status" />
              <span>Laya 决策</span>
            </label>
            <small id="laya-provider-status">自然语言 UI 用例使用上方配置的兼容 Laya 决策协议的模型。</small>
          </div>
          <div class="field">
            <label class="switch-row">
              <input v-model="form.jev_enabled" name="jev_enabled" type="checkbox" aria-describedby="jev-provider-status" />
              <span>请求 JEV 决策</span>
            </label>
            <small id="jev-provider-status">缺少已验证的服务接口。租户 UI 运行记录降级，沿用原有决策流程。</small>
          </div>
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
.field > span, .field > label:not(.switch-row) { font-size: 12px; color: var(--muted); }
.field input { width: 100%; min-height: 36px; }
.field select { font: inherit; color: var(--text); background: var(--panel); border: 1px solid var(--line); border-radius: 6px; padding: 0 10px; min-height: 36px; }
.field input:focus-visible, .field select:focus-visible, button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.field small { color: var(--muted); font-size: 12px; }
.budget-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.switch-list { display: grid; gap: 12px; }
.switch-row { display: inline-flex; align-items: center; gap: 8px; min-height: 32px; }
.switch-row input { accent-color: var(--accent); width: 16px; min-height: 16px; height: 16px; }
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
