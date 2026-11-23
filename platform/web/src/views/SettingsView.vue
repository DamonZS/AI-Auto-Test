<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import DesktopWorkerActivation from '../components/DesktopWorkerActivation.vue'

const props = defineProps({ canEdit: Boolean })
const emit = defineEmits(['open-mcp'])

const deploymentOptions = [
  { title: '桌面启动', options: [
    ['--port', '本机监听端口，默认 8090'],
    ['--data-dir', '数据库、缓存和日志目录'],
    ['--no-browser', '仅启动本机服务'],
    ['--stop / --notify', '停止本机服务并可显示结果'],
  ] },
  { title: '服务端与账号', options: [
    ['-addr / AUTOTEST_PORT', '服务端监听地址或 Compose 发布端口'],
    ['-db / -web / AUTOTEST_IMAGE', '数据库路径、界面目录或 Compose 镜像'],
    ['-desktop-setup / -desktop-worker-dir', '限制在回环访问的桌面注册和执行器配置'],
    ['-provision-user / -auth-tenant / -auth-tenant-name / -auth-role', '创建账号、选择工作区和分配角色'],
    ['PLATFORM_AUTH_USERNAME / PLATFORM_AUTH_PASSWORD', '一次性预配置账号凭据'],
  ] },
  { title: '执行器与知识库', options: [
    ['--server / --poll', '控制面地址和任务轮询间隔'],
    ['PLATFORM_WORKER_TOKEN', '执行器专用凭据'],
    ['PLATFORM_BROWSER / PLAYWRIGHT_BROWSERS_PATH', '默认浏览器及浏览器安装目录'],
    ['PLATFORM_WORKER_CACHE_DIR / PLATFORM_KNOWLEDGE_DIR', '执行缓存和 Chroma 知识库目录'],
    ['NEO4J_ENABLED / NEO4J_URI / NEO4J_USER / NEO4J_PASSWORD / NEO4J_PROJECT', '浏览器知识图谱连接和项目隔离'],
    ['OPENAI_BASE_URL / OPENAI_API_KEY / PLANNER_MODEL / EXECUTOR_MODEL', '独立 auto_ai 命令行的模型配置'],
  ] },
]

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
  if (!props.canEdit || saving.value) return
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

    <p v-if="!canEdit" class="message muted">当前工作区设置为只读，所有者可修改。</p>
    <form class="settings-form" aria-label="工作区设置" @submit.prevent="save">
      <fieldset class="section" :disabled="!canEdit || saving">
        <legend>模型连接</legend>
        <p class="section-meta">
          <span v-if="apiKeyConfigured" class="configured">API Key 已配置</span>
          <span v-else class="muted">API Key 未配置</span>
        </p>

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
      </fieldset>

      <fieldset class="section" :disabled="!canEdit || saving">
        <legend>运行预算</legend>
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
      </fieldset>

      <fieldset class="section" :disabled="!canEdit || saving">
        <legend>浏览器</legend>
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
      </fieldset>

      <fieldset class="section" :disabled="!canEdit || saving">
        <legend>增强档</legend>
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
      </fieldset>

      <footer v-if="canEdit" class="form-actions">
        <button class="primary" type="submit" :disabled="saving">{{ saving ? '保存中…' : '保存设置' }}</button>
      </footer>
    </form>

    <section class="section settings-links" aria-labelledby="integration-settings-title">
      <div>
        <h3 id="integration-settings-title">MCP 接入</h3>
        <p class="muted">服务地址、客户端配置、令牌签发与撤销</p>
      </div>
      <button type="button" @click="emit('open-mcp')">打开 MCP 接入</button>
    </section>

    <section class="section" aria-labelledby="deployment-settings-title">
      <h3 id="deployment-settings-title">部署与执行机配置</h3>
      <p class="muted">在启动命令或部署环境中配置，修改后重启对应进程。</p>
      <details v-for="group in deploymentOptions" :key="group.title" class="deployment-group">
        <summary>{{ group.title }}</summary>
        <dl>
          <div v-for="[option, description] in group.options" :key="option">
            <dt><code>{{ option }}</code></dt>
            <dd>{{ description }}</dd>
          </div>
        </dl>
      </details>
    </section>
  </div>
</template>

<style scoped>
.settings-page { display: grid; gap: 14px; max-width: 820px; }
.settings-form { display: flex; flex-direction: column; gap: 14px; }
.section { min-inline-size: 0; margin: 0; padding: 16px 20px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); }
.section legend, .section h3 { margin: 0; font-size: 14px; font-weight: 600; }
.section legend { padding-inline: 5px; }
.section-meta { margin: 0 0 12px; }
.settings-links { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; }
.settings-links p { margin: 4px 0 0; }
.deployment-group { border-top: 1px solid var(--line); padding-block: 8px; }
.deployment-group summary { cursor: pointer; }
.deployment-group dl { display: grid; gap: 8px; margin-block: 8px 0; }
.deployment-group dt { overflow-wrap: anywhere; }
.deployment-group dd { margin: 2px 0 0; color: var(--muted); }
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
