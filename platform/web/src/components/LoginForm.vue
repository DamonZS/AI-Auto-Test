<script setup>
const props = defineProps({ busy: Boolean, error: String, setupMode: Boolean })
const emit = defineEmits(['login'])

function submit(event) {
  const fields = new FormData(event.currentTarget)
  const credentials = { username: fields.get('username'), password: fields.get('password') }
  if (props.setupMode) credentials.workspace_name = fields.get('workspace_name')
  emit('login', credentials)
}
</script>

<template>
  <form class="login-form" method="post" :action="setupMode ? '/api/auth/setup' : '/api/auth/login'" @submit.prevent="submit">
    <template v-if="setupMode">
      <label for="workspace-name">工作区名称</label>
      <input id="workspace-name" name="workspace_name" autocomplete="off" required maxlength="128" />
    </template>
    <label for="username">用户名</label>
    <input id="username" name="username" autocomplete="username" autocapitalize="none" autocorrect="off" spellcheck="false" required maxlength="128" />
    <label for="password">密码</label>
    <input id="password" name="password" type="password" :autocomplete="setupMode ? 'new-password' : 'current-password'" required maxlength="1024" />
    <button v-if="setupMode" class="primary" type="submit" :disabled="busy">{{ busy ? '创建中…' : '创建工作区' }}</button>
    <button v-else class="primary" type="submit" :disabled="busy">{{ busy ? '登录中…' : '登录' }}</button>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
  </form>
</template>

<style scoped>
.login-form { display: flex; flex-direction: column; gap: 10px; }
.error { color: var(--bad); font-size: 12px; margin: 0; }
</style>
