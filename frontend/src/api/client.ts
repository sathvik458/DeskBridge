import type { Device, ServerStatus } from './types'

const base = import.meta.env.VITE_API_BASE ?? ''

export class RequestFailed extends Error {
  constructor(readonly status: number, message: string) {
    super(message)
    this.name = 'RequestFailed'
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(base + path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })

  if (!response.ok) {
    let message = `request failed with ${response.status}`
    try {
      const body = await response.json()
      if (body?.error) message = body.error
    } catch {
      // the server did not send our error shape; the status is all we have
    }
    throw new RequestFailed(response.status, message)
  }

  if (response.status === 204) return undefined as T

  return response.json() as Promise<T>
}

export const api = {
  status: () => request<ServerStatus>('/api/status'),
  devices: () => request<Device[]>('/api/devices'),
  device: (id: string) => request<Device>(`/api/devices/${encodeURIComponent(id)}`),
  heartbeat: (id: string) =>
    request<void>(`/api/devices/${encodeURIComponent(id)}/heartbeat`, { method: 'POST' }),
}
