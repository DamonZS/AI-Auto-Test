<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'

const tokenPath = '/api/auth/service-tokens'
const endpoint = new URL('/mcp', window.location.origin).href
const columns = [
  { key: 'id', label: '令牌 ID' },
  { key: 'expires_at', label: '到期时间' },
  { key: 'actions', label: '操作' },
]

const tokens = ref([])
const issued = ref(null)
const canManage = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const copyStatus = ref('')
let active = true

async function load() {
  busy.value = true
  error.value = ''
  try {
    const items = await api.get(tokenPath)
    tokens.value = items.filter((item) => item.scope === 'mcp')
    canManage.value = true
  } catch (err) {
    error.value = err.status === 403 ? '仅工作区所有者可管理 MCP 令牌。' : '读取 MCP 令牌失败，请重试。'
  } finally {
    busy.value = false
  }
}

async function issue() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const { id, scope, expires_at, token } = await api.post(tokenPath, { scope: 'mcp' })
    if (!active) return
    tokens.value = [...tokens.value, { id, scope, expires_at }]
    issued.value = { id, token }
  } catch (err) {
    error.value = err.status === 403 ? '仅工作区所有者可管理 MCP 令牌。' : '签发 MCP 令牌失败。'
  } finally {
    busy.value = false
  }
}

async function revoke(id) {
  if (!window.confirm(`撤销 MCP 令牌 ${id}？使用此令牌的客户端将无法继续连接。`)) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await api.delete(`${tokenPath}/${encodeURIComponent(id)}`)
    tokens.value = tokens.value.filter((item) => item.id !== id)
    if (issued.value?.id === id) issued.value = null
    notice.value = 'MCP 令牌已撤销。'
  } catch (err) {
    if (err.status === 404) {
      tokens.value = tokens.value.filter((item) => item.id !== id)
      if (issued.value?.id === id) issued.value = null
      notice.value = '该 MCP 令牌已不存在。'
    } else {
      error.value = err.status === 403 ? '仅工作区所有者可管理 MCP 令牌。' : '撤销 MCP 令牌失败。'
    }
  } finally {
    busy.value = false
  }
}

async function copyEndpoint() {
  try {
    await navigator.clipboard.writeText(endpoint)
    copyStatus.value = '地址已复制。'
  } catch {
    copyStatus.value = '无法访问剪贴板，请手动选择并复制地址。'
  }
}

onMounted(load)
onBeforeUnmount(() => {
  active = false
  issued.value = null
})
</script>

<template>
  <div class="mcp-page">
    <section class="section" aria-labelledby="mcp-connection-title">
      <h3 id="mcp-connection-title">客户端连接</h3>
      <dl class="connection">
        <dt>传输方式</dt>
        <dd>Streamable HTTP</dd>
        <dt>服务地址</dt>
        <dd class="endpoint"><code>{{ endpoint }}</code><button type="button" @click="copyEndpoint">复制地址</button></dd>
        <dt>请求头</dt>
        <dd><code>Authorization: Bearer &lt;MCP 令牌&gt;</code></dd>
      </dl>
      <p class="muted" role="status">{{ copyStatus }}</p>
      <details>
        <summary>Sampling 与可用工具</summary>
        <p>源码生成用例需要支持 Sampling 的客户端。客户端在 <code>initialize</code> 中声明 <code>capabilities.sampling: {}</code>，以 <code>2025-06-18</code> 协议版本和同一会话接收并回复 <code>sampling/createMessage</code>。</p>
        <p>用 <code>tools/list</code> 发现用例查询、执行状态、报告、缺陷与复测工具。<code>generate_test_cases_from_code</code> 生成的普通功能用例须在平台审核定稿后才能执行。</p>
      </details>
    </section>

    <section class="section" aria-labelledby="mcp-tokens-title">
      <header class="section-head">
        <h3 id="mcp-tokens-title">MCP 令牌</h3>
        <button v-if="canManage" class="primary" type="button" :disabled="busy || !!issued" @click="issue">签发令牌</button>
        <button v-else-if="error" type="button" :disabled="busy" @click="load">重试</button>
      </header>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p v-if="notice" role="status">{{ notice }}</p>
      <p v-if="busy && !canManage" class="muted" role="status">正在读取令牌…</p>

      <div v-if="issued" class="issued">
        <p role="status">令牌仅在本次签发时显示。请保存到客户端的凭据配置，关闭后无法再次查看。</p>
        <code class="secret" tabindex="0" aria-label="新签发的 MCP 令牌">{{ issued.token }}</code>
        <button type="button" @click="issued = null">关闭令牌</button>
      </div>

      <DataTable v-if="canManage" :rows="tokens" :columns="columns" empty="暂无 MCP 令牌">
        <template #expires_at="{ row }">{{ new Date(row.expires_at).toLocaleString() }}</template>
        <template #actions="{ row }">
          <button type="button" :disabled="busy" :aria-label="`撤销令牌 ${row.id}`" @click="revoke(row.id)">撤销</button>
        </template>
      </DataTable>
    </section>
  </div>
</template>

<style scoped>
.mcp-page { display: grid; gap: 14px; max-width: 820px; min-width: 0; }
.section { min-width: 0; padding: 18px 20px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); }
h3 { margin: 0; font-size: 14px; font-weight: 600; }
.section-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 14px; }
.connection { display: grid; grid-template-columns: max-content minmax(0, 1fr); align-items: baseline; gap: 8px 16px; }
.connection dt, .muted { color: var(--muted); }
.connection dd { margin: 0; overflow-wrap: anywhere; }
.endpoint { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
summary { cursor: pointer; }
details p:last-child { margin-bottom: 0; }
.error { color: var(--bad); }
.issued { display: grid; justify-items: start; gap: 8px; padding: 12px; margin-bottom: 12px; border: 1px solid var(--line); border-radius: 6px; }
.issued p { margin: 0; }
.secret { overflow-wrap: anywhere; user-select: all; }
button:disabled { opacity: .55; cursor: default; }
</style>
