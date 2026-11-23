<script setup>
import { computed, onMounted, ref, shallowRef } from 'vue'
import { api, setSessionExpiredHandler, setTenant } from './api'
import LoginForm from './components/LoginForm.vue'
import OverviewView from './views/OverviewView.vue'
import ProjectsView from './views/ProjectsView.vue'
import RequirementsView from './views/RequirementsView.vue'
import VersionsView from './views/VersionsView.vue'
import EnvironmentsView from './views/EnvironmentsView.vue'
import CasesView from './views/CasesView.vue'
import SuitesView from './views/SuitesView.vue'
import WorkbenchView from './views/WorkbenchView.vue'
import RunsView from './views/RunsView.vue'
import BugsView from './views/BugsView.vue'
import PatchesView from './views/PatchesView.vue'
import TestDataView from './views/TestDataView.vue'
import SettingsView from './views/SettingsView.vue'

const NAV = [
  { key: 'overview', label: '概览', comp: OverviewView },
  { key: 'projects', label: '项目', comp: ProjectsView },
  { key: 'requirements', label: '需求', comp: RequirementsView },
  { key: 'versions', label: '版本', comp: VersionsView },
  { key: 'environments', label: '环境', comp: EnvironmentsView },
  { key: 'test-data', label: '测试数据', comp: TestDataView },
  { key: 'cases', label: '用例', comp: CasesView },
  { key: 'suites', label: '套件', comp: SuitesView },
  { key: 'workbench', label: '企业流程', comp: WorkbenchView },
  { key: 'runs', label: '执行记录', comp: RunsView },
  { key: 'bugs', label: '缺陷', comp: BugsView },
  { key: 'patches', label: '自愈审批', comp: PatchesView },
  { key: 'settings', label: '设置', comp: SettingsView },
]

const signedIn = ref(false)
const checkingSetup = ref(true)
const setupAvailable = ref(false)
const tenantId = ref('')
const memberships = ref([])
const selectedTenant = ref('')
const busy = ref(false)
const draft = ref('')
const current = ref('overview')
const error = ref('')
const viewKey = shallowRef(0)

const view = computed(() => NAV.find((n) => n.key === current.value))

onMounted(async () => {
  try {
    const { available } = await api.getSetup()
    setupAvailable.value = available
  } catch {
    error.value = '无法检查首次注册状态，请刷新重试'
  } finally {
    checkingSetup.value = false
  }
})

function switchTo(key) {
  current.value = key
  viewKey.value += 1
}

function enter(id) {
  error.value = ''
  setTenant(id)
  tenantId.value = id
  switchTo('overview')
}

async function loadMemberships() {
  busy.value = true
  error.value = ''
  memberships.value = []
  selectedTenant.value = ''
  try {
    const items = await api.get('/api/tenants')
    if (signedIn.value) {
      memberships.value = items
      selectedTenant.value = items[0]?.id || ''
    }
  } catch (err) {
    if (signedIn.value) error.value = err.message
  } finally {
    busy.value = false
  }
}

async function login({ username, password, workspace_name }) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  const settingUp = setupAvailable.value
  try {
    if (settingUp) {
      await api.setup({ workspace_name, username, password })
      setupAvailable.value = false
    }
    await api.login({ username, password })
  } catch (err) {
    error.value = err.message
    busy.value = false
    return
  }
  signedIn.value = true
  await loadMemberships()
  if (settingUp && signedIn.value && memberships.value.length === 1) {
    enter(memberships.value[0].id)
  }
}

function resetWorkspace() {
  setTenant('')
  tenantId.value = ''
  draft.value = ''
  switchTo('overview')
}

function resetSession(message = '') {
  resetWorkspace()
  signedIn.value = false
  memberships.value = []
  selectedTenant.value = ''
  error.value = message
}

setSessionExpiredHandler(() => resetSession('会话已失效，请重新登录'))

async function logout() {
  busy.value = true
  error.value = ''
  try {
    await api.logout()
    resetSession()
  } catch {
    if (signedIn.value) error.value = '退出登录失败，请重试'
  } finally {
    busy.value = false
  }
}

async function createWorkspace() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const workspace = await api.post('/api/tenants', { name: draft.value.trim() || '新租户' })
    if (signedIn.value) enter(workspace.id)
  } catch (err) {
    if (signedIn.value) error.value = err.message
  } finally {
    busy.value = false
  }
}

function leave() {
  resetWorkspace()
  return loadMemberships()
}
</script>

<template>
  <div v-if="!signedIn" class="gate">
    <div class="gate-box">
      <h1>测试平台</h1>
      <p v-if="checkingSetup" role="status">加载中…</p>
      <LoginForm v-else :setup-mode="setupAvailable" :busy="busy" :error="error" @login="login" />
    </div>
  </div>

  <div v-else-if="!tenantId" class="gate">
    <div class="gate-box">
      <h1>测试平台</h1>
      <label for="workspace">工作区</label>
      <select id="workspace" v-model="selectedTenant" :disabled="busy || !memberships.length">
        <option v-if="!memberships.length" value="">{{ busy ? '加载中…' : '暂无可选工作区' }}</option>
        <option v-for="item in memberships" :key="item.id" :value="item.id">{{ item.name }}（{{ item.role }}）</option>
      </select>
      <button class="primary" :disabled="busy || !selectedTenant" @click="enter(selectedTenant)">进入</button>
      <form class="workspace-form" @submit.prevent="createWorkspace">
        <input v-model="draft" aria-label="新工作区名称" placeholder="新工作区名称" maxlength="128" :disabled="busy" />
        <button class="ghost" type="submit" :disabled="busy">新建工作区</button>
      </form>
      <button v-if="error && !memberships.length" class="ghost" :disabled="busy" @click="loadMemberships">重新加载工作区</button>
      <button class="ghost" :disabled="busy" @click="logout">退出登录</button>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
    </div>
  </div>

  <div v-else class="shell">
    <aside>
      <div class="brand">测试平台</div>
      <nav>
        <button
          v-for="n in NAV"
          :key="n.key"
          :class="{ on: current === n.key }"
          @click="switchTo(n.key)"
        >{{ n.label }}</button>
      </nav>
      <button class="leave" :disabled="busy" @click="leave">切换工作区</button>
      <button class="ghost" :disabled="busy" @click="logout">退出登录</button>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
    </aside>

    <main>
      <h2>{{ view.label }}</h2>
      <component :is="view.comp" :key="viewKey" />
    </main>
  </div>
</template>

<style scoped>
.gate { display: grid; place-items: center; min-height: 100vh; }
.gate-box { width: 300px; }
.gate-box, .workspace-form { display: flex; flex-direction: column; gap: 10px; }
.gate-box h1 { font-size: 20px; font-weight: 600; margin: 0 0 10px; }
.ghost { border: 0; background: transparent; color: var(--muted); }

.shell { display: flex; min-height: 100vh; }
aside {
  width: 196px;
  flex: 0 0 196px;
  background: var(--panel);
  border-right: 1px solid var(--line);
  padding: 18px 12px;
  display: flex;
  flex-direction: column;
}
.brand { font-weight: 600; padding: 0 8px 18px; }
nav { display: flex; flex-direction: column; gap: 2px; }
nav button {
  border: 0;
  background: transparent;
  text-align: left;
  padding: 8px 10px;
  color: var(--muted);
  border-radius: 6px;
}
nav button.on { background: #eef1fe; color: var(--accent); font-weight: 500; }
.leave { margin-top: auto; border: 0; background: transparent; color: var(--muted); text-align: left; padding: 8px 10px; font-size: 12px; }

main { flex: 1; padding: 22px 30px 40px; max-width: 1180px; }
main h2 { font-size: 17px; font-weight: 600; margin: 0 0 18px; }
.error { color: var(--bad); font-size: 12px; margin: 0; }
</style>
