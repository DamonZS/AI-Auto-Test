<script setup>
import { computed, nextTick, onMounted, ref } from 'vue'
import { api } from '../api'
import DataTable from '../components/DataTable.vue'

const props = defineProps({ projectId: { type: String, default: '' } })

const datasets = ref([])
const projects = ref([])
const environments = ref([])
const records = ref([])
const selected = ref(null)
const error = ref('')
const notice = ref('')
const mode = ref('ai')
const submitting = ref(false)
const recordsTitle = ref(null)
const form = ref({ project_id: props.projectId, environment_id: '', name: '', description: '', count: 10 })
const aiForm = ref({ prompt: '', count: 10 })
const fields = ref([
  { name: 'username', type: 'name', sensitive: false, example: '' },
  { name: 'email', type: 'email', sensitive: false, example: '' },
  { name: 'password', type: 'password', sensitive: true, example: '' },
])

const typeLabels = {
  name: '姓名', email: '邮箱', phone: '手机号', number: '数字', date: '日期',
  uuid: 'UUID', text: '文本', password: '密码',
}

const presets = [
  { label: '客户信息', keys: ['customer_name', 'customer_id', 'document_type', 'document_number', 'company_name', 'birth_date', 'member_level'] },
  { label: '登录认证', keys: ['username', 'password', 'email', 'phone', 'account_status'] },
  { label: '联系地址', keys: ['contact_name', 'phone', 'province', 'city', 'address', 'postal_code'] },
  { label: '订单交易', keys: ['order_number', 'amount', 'quantity', 'order_date', 'order_status'] },
]
const aiFieldLabels = {
  customer_name: '客户姓名', customer_id: '客户编号', username: '用户名', password: '密码',
  email: '邮箱', phone: '手机号', document_type: '证件类型', document_number: '证件号码',
  company_name: '公司名称', contact_name: '联系人', province: '省份', city: '城市', address: '详细地址',
  postal_code: '邮政编码', birth_date: '出生日期', member_level: '会员等级', account_status: '账号状态',
  order_number: '订单编号', amount: '订单金额', quantity: '商品数量', order_date: '下单日期', order_status: '订单状态',
}
const aiFields = ref(new Set(presets.slice(0, 2).flatMap((preset) => preset.keys)))

const projectEnvironments = computed(() => environments.value.filter((environment) => environment.project_id === form.value.project_id))
const selectedFields = computed(() => selected.value?.fields || [])
const visibleDatasets = computed(() => props.projectId ? datasets.value.filter((dataset) => dataset.project_id === props.projectId) : datasets.value)

function presetChecked(preset) {
  return preset.keys.every((key) => aiFields.value.has(key))
}

function togglePreset(preset, checked) {
  const sharedKeys = new Set(presets.filter((item) => item !== preset && presetChecked(item)).flatMap((item) => item.keys))
  for (const key of preset.keys) {
    if (checked) aiFields.value.add(key)
    else if (!sharedKeys.has(key)) aiFields.value.delete(key)
  }
}

async function load() {
  ;[projects.value, environments.value, datasets.value] = await Promise.all([
    api.get('/api/projects'),
    api.get('/api/environments'),
    api.get('/api/test-data/datasets'),
  ])
  if (!form.value.project_id && projects.value.length) form.value.project_id = projects.value[0].id
}

function addField() {
  fields.value.push({ name: '', type: 'text', sensitive: false, example: '' })
}

function removeField(index) {
  if (fields.value.length > 1) fields.value.splice(index, 1)
}

async function openCreatedRecords(id) {
  await load()
  const dataset = datasets.value.find((item) => item.id === id)
  if (!dataset) throw new Error('数据已保存，但未能在列表中找到新数据集')
  await showRecords(dataset)
  await nextTick()
  if (selected.value?.id === id) recordsTitle.value?.focus()
}

async function generateWithAI() {
  if (submitting.value) return
  error.value = ''
  notice.value = ''
  const prompt = aiForm.value.prompt.trim()
  if (!aiFields.value.size && !prompt) {
    error.value = '请至少勾选一个字段，或填写测试数据需求'
    return
  }
  submitting.value = true
  try {
    const created = await api.post('/api/test-data/generate', {
      project_id: form.value.project_id,
      environment_id: form.value.environment_id,
      ...(prompt && { prompt }),
      count: aiForm.value.count,
      fields: [...aiFields.value],
    })
    notice.value = `已保存「${created.name}」，共 ${created.record_count} 条记录`
    await openCreatedRecords(created.id)
  } catch (e) {
    error.value = e.message
  } finally {
    submitting.value = false
  }
}

async function createAndGenerate() {
  error.value = ''
  notice.value = ''
  if (!form.value.project_id || !form.value.name || fields.value.some((field) => !field.name.trim())) {
    error.value = '请填写项目、数据集名称和所有字段名'
    return
  }
  try {
    const created = await api.post('/api/test-data/datasets', {
      ...form.value,
      count: undefined,
      fields: fields.value,
    })
    await api.post(`/api/test-data/datasets/${created.id}/generate`, { count: Number(form.value.count) })
    notice.value = '数据集已创建并生成测试数据'
    form.value.name = ''
    await openCreatedRecords(created.id)
  } catch (e) { error.value = e.message }
}

async function generate(dataset) {
  const count = Number(window.prompt('生成多少条数据？', dataset.record_count || 10))
  if (!Number.isInteger(count) || count < 1 || count > 10000) return
  error.value = ''
  try {
    await api.post(`/api/test-data/datasets/${dataset.id}/generate`, { count })
    notice.value = `已生成 ${count} 条测试数据`
    await load()
    if (selected.value?.id === dataset.id) await showRecords(dataset)
  } catch (e) { error.value = e.message }
}

async function showRecords(dataset) {
  try {
    records.value = await api.get(`/api/test-data/datasets/${dataset.id}/records`)
    selected.value = dataset
  } catch (e) { error.value = e.message }
}

function projectName(id) {
  return projects.value.find((item) => item.id === id)?.name || '—'
}

onMounted(load)
</script>

<template>
  <div class="layout">
    <section class="create-panel" aria-labelledby="create-data-title">
      <header class="section-head">
        <div>
          <h3 id="create-data-title">生成测试数据</h3>
        </div>
      </header>
      <form class="data-form" @submit.prevent="mode === 'ai' ? generateWithAI() : createAndGenerate()">
        <fieldset class="mode-options" :disabled="submitting">
          <legend class="sr-only">生成方式</legend>
          <label class="check"><input v-model="mode" type="radio" name="generation_mode" value="ai" /> <span>AI 生成</span></label>
          <label class="check"><input v-model="mode" type="radio" name="generation_mode" value="manual" /> <span>手动按规则生成</span></label>
        </fieldset>
        <div class="form-grid">
          <label>
            <span>项目</span>
            <select v-model="form.project_id" name="project_id" required :disabled="!!projectId" @change="form.environment_id = ''">
              <option value="" disabled>选择项目</option>
              <option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option>
            </select>
          </label>
          <label>
            <span>绑定环境</span>
            <select v-model="form.environment_id" name="environment_id">
              <option value="">不绑定环境</option>
              <option v-for="environment in projectEnvironments" :key="environment.id" :value="environment.id">{{ environment.name }}</option>
            </select>
          </label>
          <label v-if="mode === 'manual'">
            <span>数据集名称</span>
            <input v-model="form.name" name="name" required maxlength="80" placeholder="例如：登录回归数据" />
          </label>
          <label v-if="mode === 'ai'">
            <span>生成数量（1–30）</span>
            <input v-model.number="aiForm.count" name="count" type="number" min="1" max="30" required />
          </label>
          <label v-else>
            <span>生成数量</span>
            <input v-model.number="form.count" name="count" type="number" min="1" max="10000" required />
          </label>
        </div>
        <template v-if="mode === 'ai'">
          <fieldset>
            <legend>字段预设</legend>
            <div class="preset-options">
              <label v-for="preset in presets" :key="preset.label" class="check">
                <input
                  type="checkbox" name="preset" :value="preset.label" :checked="presetChecked(preset)"
                  :indeterminate="!presetChecked(preset) && preset.keys.some((key) => aiFields.has(key))"
                  @change="togglePreset(preset, $event.target.checked)"
                />
                <span>{{ preset.label }}</span>
              </label>
            </div>
          </fieldset>
          <fieldset>
            <legend>生成字段</legend>
            <div class="field-options">
              <label v-for="(label, key) in aiFieldLabels" :key="key" class="check">
                <input v-model="aiFields" type="checkbox" name="fields" :value="key" />
                <span>{{ label }}</span>
              </label>
            </div>
          </fieldset>
          <label class="wide">
            <span>测试数据需求（可选）</span>
            <textarea v-model="aiForm.prompt" name="prompt" rows="2" placeholder="例如：账号状态覆盖正常、锁定和停用，订单金额包含零值和小数。" />
          </label>
          <div class="form-footer">
            <button type="submit" class="primary" :disabled="submitting">生成并保存</button>
            <span v-if="submitting" class="hint" role="status">正在生成并保存…</span>
          </div>
        </template>
        <template v-else>
          <label class="wide">
            <span>说明</span>
            <textarea v-model="form.description" name="description" rows="2" maxlength="300" placeholder="说明这份数据服务哪些场景" />
          </label>
          <fieldset>
            <legend>字段规则</legend>
            <div v-for="(field, index) in fields" :key="index" class="field-row">
              <label>
                <span>字段名</span>
                <input v-model="field.name" :name="`field-${index}-name`" required placeholder="username" />
              </label>
              <label>
                <span>类型</span>
                <select v-model="field.type" :name="`field-${index}-type`">
                  <option v-for="(label, type) in typeLabels" :key="type" :value="type">{{ label }}</option>
                </select>
              </label>
              <label class="example">
                <span>前缀/示例</span>
                <input v-model="field.example" :name="`field-${index}-example`" placeholder="可选" />
              </label>
              <label class="check">
                <input v-model="field.sensitive" :name="`field-${index}-sensitive`" type="checkbox" />
                <span>敏感字段</span>
              </label>
              <button type="button" class="remove" :aria-label="`删除字段 ${field.name || index + 1}`" @click="removeField(index)">删除</button>
            </div>
            <button type="button" class="secondary" @click="addField">添加字段</button>
          </fieldset>
          <div class="form-footer">
            <span class="hint">敏感字段只在执行端接口返回原值，列表与报告默认掩码。</span>
            <button type="submit" class="primary">创建并生成</button>
          </div>
        </template>
      </form>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    </section>

    <section aria-labelledby="datasets-title">
      <header class="section-head">
        <div>
          <h3 id="datasets-title">数据集</h3>
          <p>数据按租户和项目隔离，执行记录引用生成时的具体数据集。</p>
        </div>
      </header>
      <DataTable
        :rows="visibleDatasets"
        :columns="[
          { key: 'name', label: '数据集', width: '20%' },
          { key: 'project_name', label: '项目', width: '16%' },
          { key: 'record_count', label: '记录数', width: '10%' },
          { key: 'fields', label: '字段' },
          { key: 'updated_at', label: '更新时间', width: '18%' },
          { key: 'actions', label: '', width: '22%' },
        ]"
        empty="还没有测试数据集"
      >
        <template #name="{ row }">
          {{ row.name }} <span v-if="row.generation_source === 'ai'" class="chip">AI 生成</span>
        </template>
        <template #fields="{ row }">
          <span v-for="field in row.fields" :key="field.name" class="chip">{{ field.name }}<b v-if="field.sensitive">敏感</b></span>
        </template>
        <template #actions="{ row }">
          <div class="actions">
            <button @click="showRecords(row)">查看数据</button>
            <button v-if="row.generation_source !== 'ai'" @click="generate(row)">重新生成</button>
          </div>
        </template>
      </DataTable>
    </section>

    <section v-if="selected" class="records-panel" aria-labelledby="records-title">
      <header class="section-head">
        <div>
          <h3 id="records-title" ref="recordsTitle" tabindex="-1">{{ selected.name }} · 数据预览</h3>
          <p>这里只展示脱敏值；执行端通过设备令牌领取原始值。</p>
        </div>
      </header>
      <DataTable
        :rows="records"
        :columns="[
          { key: 'row_no', label: '#', width: '8%' },
          ...selectedFields.map((field) => ({ key: field.name, label: field.name })),
        ]"
        empty="该数据集还没有记录"
      >
        <template v-for="field in selectedFields" :key="field.name" #[field.name]="{ row }">{{ row.values?.[field.name] || '—' }}</template>
      </DataTable>
    </section>
  </div>
</template>

<style scoped>
.layout { display: flex; flex-direction: column; gap: 24px; }
.create-panel, .records-panel { background: var(--panel); border: 1px solid var(--line); border-radius: 10px; padding: 18px; }
.section-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
.section-head h3 { margin: 0; font-size: 14px; font-weight: 600; }
.section-head p { margin: 4px 0 0; color: var(--muted); font-size: 12px; }
.data-form { display: flex; flex-direction: column; gap: 16px; }
.form-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
label { display: flex; flex-direction: column; gap: 5px; min-width: 0; }
label > span { color: var(--muted); font-size: 12px; }
input, select, textarea { font: inherit; font-size: max(1rem, 14px); min-height: 40px; border: 1px solid #b7b7bf; border-radius: 6px; padding: 8px 10px; background: var(--panel); color: var(--text); }
textarea { resize: vertical; }
input:focus-visible, select:focus-visible, textarea:focus-visible, button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.wide { max-width: 720px; }
fieldset { border: 1px solid var(--line); border-radius: 8px; padding: 12px; }
legend { color: var(--muted); font-size: 12px; padding: 0 5px; }
.field-row { display: grid; grid-template-columns: 1.1fr 1fr 1.2fr auto auto; gap: 10px; align-items: end; margin-bottom: 10px; }
.check { flex-direction: row; align-items: center; gap: 6px; padding-bottom: 9px; white-space: nowrap; }
.check input { min-height: auto; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
.mode-options, .preset-options, .field-options { display: flex; flex-wrap: wrap; gap: 8px 16px; }
.mode-options { border: 0; padding: 0; margin: 0; }
.mode-options .check, .preset-options .check, .field-options .check { padding: 0; }
.field-options { display: grid; grid-template-columns: repeat(auto-fill, minmax(105px, 1fr)); }
.remove { margin-bottom: 4px; color: var(--bad); }
.secondary { background: #f7f7f9; }
.form-footer { display: flex; justify-content: space-between; gap: 16px; align-items: center; }
.form-footer button:disabled { opacity: .55; cursor: wait; }
.hint { color: var(--muted); font-size: 12px; }
.actions { display: flex; gap: 6px; }
.actions button { padding: 4px 9px; font-size: 12px; }
.chip { display: inline-flex; align-items: center; gap: 4px; background: #f1f1f4; border-radius: 4px; padding: 2px 7px; margin: 2px 4px 2px 0; font-size: 12px; }
.chip b { font-size: 10px; font-weight: 500; color: var(--bad); }
.error { color: var(--bad); font-size: 12px; margin: 0; }
.notice { color: var(--ok); font-size: 12px; margin: 0; }
@media (max-width: 900px) { .form-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .field-row { grid-template-columns: 1fr 1fr; } .example { grid-column: span 2; } .form-footer { align-items: flex-start; flex-direction: column; } }
</style>
