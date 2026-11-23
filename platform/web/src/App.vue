<script setup>
import { computed, onMounted, ref, shallowRef } from 'vue'
import { api, setSessionExpiredHandler, setTenant } from './api'
import { closeSelectPicker } from './selectPicker'
import AppIcon from './components/AppIcon.vue'
import LoginForm from './components/LoginForm.vue'
import ProjectJourney from './components/ProjectJourney.vue'
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
import McpView from './views/McpView.vue'

const NAV = [
  { key: 'overview', label: '概览', comp: OverviewView },
  { key: 'projects', label: '项目', comp: ProjectsView },
  { key: 'requirements', label: '需求', comp: RequirementsView },
  { key: 'versions', label: '版本', comp: VersionsView },
  { key: 'environments', label: '环境', comp: EnvironmentsView },
  { key: 'test-data', label: '测试数据', comp: TestDataView },
  { key: 'cases', label: '用例', comp: CasesView },
  { key: 'suites', label: '套件', comp: SuitesView },
  { key: 'workbench', label: '流程与报告', comp: WorkbenchView },
  { key: 'runs', label: '执行记录', comp: RunsView },
  { key: 'bugs', label: '缺陷', comp: BugsView },
  { key: 'patches', label: '自愈审批', comp: PatchesView },
  { key: 'mcp', label: 'MCP 接入', comp: McpView },
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
const selectedRun = ref('')
const selectedCase = ref('')
const generationTarget = ref(null)
const projects = ref([])
const projectId = ref('')
const projectPages = new Set(['requirements', 'versions', 'environments', 'test-data', 'cases', 'suites', 'workbench', 'runs', 'bugs'])
const navGroups = [
  { label: '工作区', keys: ['overview', 'projects'] },
  { label: '测试准备', keys: ['requirements', 'versions', 'environments', 'test-data', 'cases', 'suites'] },
  { label: '质量交付', keys: ['runs', 'bugs', 'workbench', 'patches'] },
  { label: '平台', keys: ['mcp', 'settings'] },
].map((group) => ({ label: group.label, pages: group.keys.map((key) => NAV.find((item) => item.key === key)) }))

const view = computed(() => NAV.find((n) => n.key === current.value))
const viewProps = computed(() => {
  const scope = { projectId: projectId.value }
  if (current.value === 'runs') return { ...scope, selectedRun: selectedRun.value }
  if (current.value === 'cases') return { ...scope, generationTarget: generationTarget.value, selectedCase: selectedCase.value }
  if (current.value === 'patches') return { selectedRun: selectedRun.value }
  return projectPages.has(current.value) ? scope : {}
})

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
  closeSelectPicker()
  selectedRun.value = ''
  selectedCase.value = ''
  generationTarget.value = null
  current.value = key
  viewKey.value += 1
}

async function refreshProjects() {
  const workspace = tenantId.value
  if (!workspace) return
  try {
    const items = await api.get('/api/projects')
    if (tenantId.value !== workspace) return
    projects.value = items
  } catch (problem) {
    if (tenantId.value === workspace) error.value = problem.message
  }
}

function openProject(id) {
  projectId.value = id
  switchTo('requirements')
  refreshProjects()
}

function openProjectView(target) {
  if (!NAV.some((item) => item.key === target.view)) return
  if (target.projectId) projectId.value = target.projectId
  switchTo(target.view)
  if (target.view === 'patches') selectedRun.value = target.runId || ''
  refreshProjects()
}

function selectProject() {
  if (!projectPages.has(current.value)) switchTo(projectId.value ? 'requirements' : 'projects')
  else switchTo(current.value)
}

function openRun(id, sourceProject) {
  if (sourceProject) projectId.value = sourceProject
  switchTo('runs')
  selectedRun.value = id
}

function openCaseHistory(target) {
  if (target.projectId) projectId.value = target.projectId
  switchTo('cases')
  selectedCase.value = target.caseId
}

function generateCases(target) {
  if (target.projectId) projectId.value = target.projectId
  switchTo('cases')
  generationTarget.value = target
}

function enter(id) {
  error.value = ''
  setTenant(id)
  tenantId.value = id
  projects.value = []
  projectId.value = ''
  switchTo('overview')
  refreshProjects()
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
  projects.value = []
  projectId.value = ''
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
      <div class="brand"><span class="brand-mark" aria-hidden="true"><AppIcon name="overview" /></span>测试平台</div>
      <nav aria-label="工作台导航">
        <div v-for="group in navGroups" :key="group.label" class="nav-group">
          <span>{{ group.label }}</span>
          <button
          v-for="n in group.pages"
          :key="n.key"
          type="button"
          :class="{ on: current === n.key }"
          :aria-current="current === n.key ? 'page' : undefined"
          @click="switchTo(n.key)"
        >
          <AppIcon :name="n.key" />
          <span>{{ n.label }}</span>
          </button>
        </div>
      </nav>
      <footer class="workspace-actions">
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <button type="button" :disabled="busy" @click="leave">切换工作区</button>
        <button type="button" :disabled="busy" @click="logout">退出登录</button>
      </footer>
    </aside>

    <main>
      <header class="workbench-header">
        <h2>{{ view.label }}</h2>
        <label v-if="projects.length" class="scope-switch">当前项目
          <select v-model="projectId" @change="selectProject">
            <option value="">全部项目</option>
            <option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option>
          </select>
        </label>
        <button v-else-if="current !== 'projects'" type="button" @click="switchTo('projects')">进入项目管理</button>
      </header>
      <ProjectJourney v-if="projectId && projectPages.has(current)" :page="current" @navigate="switchTo" />
      <div :key="viewKey" class="workbench-page">
        <component :is="view.comp" v-bind="viewProps" @open-run="openRun" @generate-cases="generateCases"
          @open-project="openProject" @open-project-view="openProjectView" @open-case-history="openCaseHistory" @projects-changed="refreshProjects" />
      </div>
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
  align-self: flex-start;
  position: sticky;
  top: 0;
  height: 100dvh;
  background: var(--panel);
  border-right: 1px solid var(--line);
  padding: 18px 12px;
  display: flex;
  flex-direction: column;
}
.brand { display: flex; flex-shrink: 0; align-items: center; gap: 9px; font-weight: 700; padding: 0 8px 18px; }
.brand-mark { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 9px; color: white; background: var(--accent); }
aside nav { display: flex; flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; flex-direction: column; gap: 12px; padding: 3px; }
.nav-group { display: grid; gap: 2px; }
.nav-group > span { padding: 0 10px 4px; color: var(--muted); font-size: 11px; }
nav button {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 0;
  background: transparent;
  text-align: left;
  padding: 8px 10px;
  color: var(--muted);
  border-radius: 6px;
}
nav button:hover { background: var(--bg); color: var(--text); }
nav button.on { background: var(--accent-soft); color: var(--accent); font-weight: 600; }
.workspace-actions { display: grid; flex-shrink: 0; gap: 2px; margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--line); }
.workspace-actions button { border: 0; background: transparent; color: var(--muted); text-align: left; padding: 6px 10px; }
.workspace-actions button:enabled:hover { background: var(--bg); color: var(--text); }

main { flex: 1; min-width: 0; padding: 24px clamp(16px, 2.4vw, 36px) 40px; max-width: 1500px; }
.workbench-header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; margin-bottom: 18px; }
.workbench-header h2 { font-size: 22px; font-weight: 700; margin: 0; }
.scope-switch { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--muted); }
.scope-switch select { max-width: 200px; }
.error { color: var(--bad); font-size: 12px; margin: 0; }
@media (max-width: 740px) {
  aside { width: 130px; flex-basis: 130px; padding-inline: 6px; }
  nav button { padding-inline: 7px; }
}
</style>
