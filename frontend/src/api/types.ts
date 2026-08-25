export type DeviceKind = 'server' | 'laptop' | 'phone' | 'desktop'
export type DeviceStatus = 'online' | 'offline' | 'unknown'

export interface Device {
  id: string
  user_id: string | null
  name: string
  kind: DeviceKind
  status: DeviceStatus
  last_seen_at: string | null
  created_at: string
  updated_at: string
}

export interface ServerStatus {
  status: string
  version: string
  started_at: string
  uptime_seconds: number
  uptime: string
  go_version: string
  platform: string
}

export interface ApiError {
  error: string
}
