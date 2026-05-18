import axios from 'axios'

const TOKEN_KEY = 'autorestic.authToken'

const client = axios.create({
	baseURL: '/api/v1',
	timeout: 0
})

export function getAuthToken(): string {
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function setAuthToken(token: string) {
  const trimmed = token.trim()
  if (trimmed) {
    localStorage.setItem(TOKEN_KEY, trimmed)
  } else {
    localStorage.removeItem(TOKEN_KEY)
  }
}

client.interceptors.request.use((config) => {
  const token = getAuthToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

export async function wsUrl(executionId: number): Promise<string> {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const { data } = await client.post(`/ws/executions/${executionId}/ticket`)
  const ticket = data?.ticket ? String(data.ticket) : ''
  const query = ticket ? `?ticket=${encodeURIComponent(ticket)}` : ''
  return `${protocol}//${window.location.host}/ws/executions/${executionId}${query}`
}

export default client
