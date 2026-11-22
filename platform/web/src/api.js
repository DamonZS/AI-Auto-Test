const KEY = 'platform.tenant'

export const getTenant = () => localStorage.getItem(KEY) || ''
export const setTenant = (id) => localStorage.setItem(KEY, id)

async function request(path, options = {}) {
  const res = await fetch(path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'X-Tenant-Id': getTenant(),
      ...options.headers,
    },
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(body.slice(0, 200) || res.statusText)
  }
  return res.json()
}

export const api = {
  get: (path) => request(path),
  post: (path, body) => request(path, { method: 'POST', body: JSON.stringify(body) }),
  patch: (path, body) => request(path, { method: 'PATCH', body: JSON.stringify(body) }),
  put: (path, body) => request(path, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (path) => request(path, { method: 'DELETE' }),
}
