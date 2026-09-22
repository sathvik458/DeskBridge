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

export type SessionStatus = 'active' | 'paused' | 'completed' | 'abandoned'

export interface Session {
  id: string
  subject: string
  topic: string | null
  status: SessionStatus
  started_at: string
  ended_at: string | null
  elapsed_seconds: number
  server_time: string
}

export interface Goal {
  id: string
  subject: string
  topic: string | null
  target_minutes: number
  goal_date: string
  done: boolean
  completed_at: string | null
  session_id: string | null
}

export type MessageKind = 'message' | 'help_request'
export type Sender = 'supporter' | 'student'

export interface Message {
  id: string
  from: Sender
  kind: MessageKind
  body: string
  session_id: string | null
  read: boolean
  created_at: string
}

export type Shelf = 'notes' | 'homework' | 'images' | 'documents' | 'resources' | 'other'

export const shelves: Shelf[] = ['notes', 'homework', 'images', 'documents', 'resources', 'other']

export interface SharedFile {
  id: string
  name: string
  category: Shelf
  size_bytes: number
  checksum: string
  created_at: string
}

export interface IntegrityCheck {
  id: string
  intact: boolean
  expected: string
  found: string
}

export type MarkKind = 'draw' | 'erase' | 'clear'

export const pens = ['#1E1E1E', '#C84B31', '#3B6E5B', '#1F4257'] as const
export type Pen = (typeof pens)[number]

export interface Mark {
  seq: number
  id: string
  kind: MarkKind
  by: Sender
  target_id?: string
  ink?: Pen
  thickness?: number
  path?: number[]
  created_at: string
}

export interface BoardPage {
  marks: Mark[]
  cursor: number
  more: boolean
}

export interface Me {
  id: string
  username: string
  display_name: string
  role: Sender
}

export interface StudyDay {
  date: string
  seconds: number
  sessions: number
}

export interface SubjectTotal {
  name: string
  seconds: number
  sessions: number
}

export interface Insights {
  days: StudyDay[]
  subjects: SubjectTotal[]
  total_seconds: number
  best_day: StudyDay
  streak: number
  session_count: number
  goals_set: number
  goals_done: number
  zone: string
}

export type FocusKind = 'focus' | 'short_break' | 'long_break'

export interface FocusRun {
  running: boolean
  id: string
  kind: FocusKind
  session_id: string | null
  planned_seconds: number
  left_seconds: number
  started_at: string
  server_time: string
  done_today: number
  into_round: number
  up_next: FocusKind
}

export interface FocusIdle {
  running: boolean
  done_today: number
  into_round: number
  up_next: FocusKind
}

export type FocusState = FocusRun | FocusIdle

export interface Notice {
  id: string
  kind: string
  line: string
  times: number
  made_at: string
  seen: boolean
}

export interface NoticeList {
  notices: Notice[]
  unseen: number
}
