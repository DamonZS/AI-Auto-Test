let sessionToken = ''
let tenantId = ''
let onSessionExpired = () => {}

export const setTenant = (id) => { tenantId = id }
export const setSessionExpiredHandler = (handler) => { onSessionExpired = handler }

function clearSession() {
  sessionToken = ''
  tenantId = ''
}

async function request(path, options = {}, authenticated = true, responseType = 'json') {
  const token = authenticated ? sessionToken : ''
  const res = await fetch(path, {
    ...options,
    headers: {
      ...(!(options.body instanceof FormData) && { 'Content-Type': 'application/json' }),
      ...(token && { Authorization: `Bearer ${token}` }),
      ...(token && tenantId && { 'X-Tenant-Id': tenantId }),
      ...options.headers,
    },
  })
  if (res.status === 401 && token && token === sessionToken) {
    clearSession()
    onSessionExpired()
  }
  if (!res.ok) {
    const body = await res.text()
    let detail
    try { detail = JSON.parse(body) } catch { detail = null }
    throw Object.assign(new Error(detail?.error || body.slice(0, 200) || res.statusText), { detail, status: res.status })
  }
  if (responseType === 'blob') return res.blob()
  return res.status === 204 || responseType === 'none' ? undefined : res.json()
}

export const api = {
  async getSetup() {
    try {
      return await request('/api/auth/setup', {}, false)
    } catch (err) {
      if (err.status === 404) return { available: false }
      throw err
    }
  },
  setup: (body) => request('/api/auth/setup', { method: 'POST', body: JSON.stringify(body) }, false, 'none'),
  async login(credentials) {
    const { token } = await request('/api/auth/login', {
      method: 'POST', body: JSON.stringify(credentials),
    }, false)
    sessionToken = token
  },
  async logout() {
    await request('/api/auth/logout', { method: 'POST' })
    clearSession()
  },
  get: (path) => request(path),
  download: (path) => request(path, {}, true, 'blob'),
  post: (path, body) => request(path, { method: 'POST', body: JSON.stringify(body) }),
  upload: (path, body) => request(path, { method: 'POST', body }),
  patch: (path, body) => request(path, { method: 'PATCH', body: JSON.stringify(body) }),
  put: (path, body) => request(path, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (path) => request(path, { method: 'DELETE' }),
}
