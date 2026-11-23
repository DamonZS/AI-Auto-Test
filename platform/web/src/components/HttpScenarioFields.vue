<script setup>
import { computed, ref } from 'vue'

const props = defineProps({ type: { type: String, required: true }, readonly: Boolean })
const scenario = defineModel({ type: String, required: true })
const raw = ref(false)
const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
const parsed = computed(() => {
  if (!scenario.value.trim()) return { request: { method: 'GET', path: '' } }
  try {
    const config = JSON.parse(scenario.value)
    if (config && typeof config === 'object' && !Array.isArray(config)) {
      if (config.request && typeof config.request === 'object' && !Array.isArray(config.request)) return config
      if (props.type === 'perf' && typeof config.path === 'string') return config
    }
  } catch {
    return null
  }
  return null
})
const request = computed(() => parsed.value?.request || parsed.value || {})
const assertions = computed(() => Array.isArray(parsed.value?.assertions) ? parsed.value.assertions : [])
const loadParameters = [
  { key: 'concurrency', label: '并发数', value: 1, min: 1, max: 100, step: 1 },
  { key: 'duration_seconds', label: '时长（秒）', value: 10, min: 0.1, max: 300, step: 0.1 },
  { key: 'max_requests', label: '请求上限', value: 100, min: 1, max: 10000, step: 1 },
  { key: 'timeout', label: '请求超时（秒）', value: 10, min: 0.001, max: 60, step: 0.001 },
]

function updateConfig(fields) {
  scenario.value = JSON.stringify({ ...parsed.value, ...fields }, null, 2)
}

function updateRequest(fields) {
  if (parsed.value.request) updateConfig({ request: { ...request.value, ...fields } })
  else updateConfig(fields)
}

function updateAssertion(index, assertion) {
  updateConfig({ assertions: assertions.value.map((item, offset) => offset === index ? assertion : item) })
}

function changeAssertionType(index, type) {
  const defaults = {
    status_code: { expected: 200 },
    json_exists: { path: '' },
    json_equals: { path: '', expected: '' },
    response_time: { max_ms: 1000 },
  }
  updateAssertion(index, { type, ...defaults[type] })
}

function numberField(event, update) {
  if (event.target.validity.valid && event.target.value !== '') update(event.target.valueAsNumber)
}
</script>

<template>
  <fieldset class="http-scenario" :disabled="readonly">
    <legend>{{ type === 'perf' ? '性能场景' : '接口场景' }}</legend>
    <label v-if="parsed"><input v-model="raw" type="checkbox" /> 编辑场景 JSON</label>
    <label v-if="raw || !parsed" class="field">场景 JSON
      <textarea v-model="scenario" rows="7" name="http-scenario-json" spellcheck="false" />
    </label>
    <template v-else>
      <div class="fields">
        <label class="field">请求方法
          <select :value="request.method || 'GET'" @change="updateRequest({ method: $event.target.value })">
            <option v-for="method in methods" :key="method">{{ method }}</option>
          </select>
        </label>
        <label class="field">请求路径
          <input :value="request.path" required placeholder="/api/items" @input="updateRequest({ path: $event.target.value })" />
        </label>
      </div>
      <div v-if="type === 'perf'" class="fields">
        <label v-for="parameter in loadParameters" :key="parameter.key" class="field">{{ parameter.label }}
          <input type="number" :min="parameter.min" :max="parameter.max" :step="parameter.step" required
            :value="parsed[parameter.key] ?? parameter.value" @change="numberField($event, (value) => updateConfig({ [parameter.key]: value }))" />
        </label>
      </div>
      <template v-else>
        <div v-for="(assertion, index) in assertions" :key="index" class="fields">
          <label class="field">断言类型
            <select :value="assertion.type" @change="changeAssertionType(index, $event.target.value)">
              <option value="status_code">状态码</option><option value="json_exists">JSON 字段存在</option>
              <option value="json_equals">JSON 字段相等</option><option value="response_time">响应时间</option>
            </select>
          </label>
          <label v-if="assertion.type === 'status_code'" class="field">期望状态码
            <input type="number" min="100" max="599" required :value="assertion.expected"
              @change="numberField($event, (expected) => updateAssertion(index, { ...assertion, expected }))" />
          </label>
          <label v-if="assertion.type === 'json_exists' || assertion.type === 'json_equals'" class="field">JSON 路径
            <input :value="assertion.path" required @input="updateAssertion(index, { ...assertion, path: $event.target.value })" />
          </label>
          <label v-if="assertion.type === 'json_equals'" class="field">期望值（JSON）
            <input :value="JSON.stringify(assertion.expected)" @change="($event) => {
              try { updateAssertion(index, { ...assertion, expected: JSON.parse($event.target.value) }) }
              catch { updateAssertion(index, { ...assertion, expected: $event.target.value }) }
            }" />
          </label>
          <label v-if="assertion.type === 'response_time'" class="field">最长响应（毫秒）
            <input type="number" min="1" required :value="assertion.max_ms"
              @change="numberField($event, (max_ms) => updateAssertion(index, { ...assertion, max_ms }))" />
          </label>
          <button v-if="!readonly" type="button" @click="updateConfig({ assertions: assertions.filter((item, offset) => offset !== index) })">移除断言</button>
        </div>
        <button v-if="!readonly" type="button" @click="updateConfig({ assertions: [...assertions, { type: 'status_code', expected: 200 }] })">添加断言</button>
      </template>
    </template>
  </fieldset>
</template>

<style scoped>
.http-scenario { display: grid; gap: 10px; margin: 12px 0; padding: 10px 12px; min-width: 0; border: 1px solid var(--line); border-radius: 6px; }
.fields { display: flex; gap: 8px; flex-wrap: wrap; align-items: end; }
.field { display: grid; gap: 4px; }
.field :is(input, select, textarea) { min-width: 0; font: inherit; color: inherit; padding: 7px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
textarea { width: 100%; resize: vertical; }
</style>
