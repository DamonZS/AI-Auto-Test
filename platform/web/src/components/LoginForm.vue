<script setup>
defineProps({ busy: Boolean, error: String })
const emit = defineEmits(['login'])

function submit(event) {
  const fields = new FormData(event.currentTarget)
  emit('login', { username: fields.get('username'), password: fields.get('password') })
}
</script>

<template>
  <form class="login-form" @submit.prevent="submit">
    <label for="username">用户名</label>
    <input id="username" name="username" autocomplete="username" autocapitalize="none" autocorrect="off" spellcheck="false" required maxlength="128" />
    <label for="password">密码</label>
    <input id="password" name="password" type="password" autocomplete="current-password" required maxlength="1024" />
    <button class="primary" type="submit" :disabled="busy">{{ busy ? '登录中…' : '登录' }}</button>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
  </form>
</template>

<style scoped>
.login-form { display: flex; flex-direction: column; gap: 10px; }
.error { color: var(--bad); font-size: 12px; margin: 0; }
</style>
