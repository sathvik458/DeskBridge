import type {
  Device,
  ServerStatus,
  Session,
  Goal,
  Message,
  MessageKind,
  Sender,
  SharedFile,
  IntegrityCheck,
  Shelf,
  Mark,
  BoardPage,
  Pen,
  Me,
  Insights,
  FocusState,
  FocusRun,
  FocusIdle,
  FocusKind,
  NoticeList,
} from './types'

const base = import.meta.env.VITE_API_BASE ?? ''

export class RequestFailed extends Error {
  constructor(readonly status: number, message: string) {
    super(message)
    this.name = 'RequestFailed'
  }
}

const guardCookie = 'deskbridge_guard'
const guardHeader = 'X-Deskbridge-Guard'

// The server sets this cookie deliberately readable so it can be copied into a
// header. A page on another origin can make the browser send the cookie but
// cannot read it, which is the whole of the CSRF defence.
function guard(): string {
  const hit = document.cookie.split('; ').find((c) => c.startsWith(guardCookie + '='))
  return hit ? decodeURIComponent(hit.slice(guardCookie.length + 1)) : ''
}

const safe = new Set(['GET', 'HEAD', 'OPTIONS'])

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? 'GET').toUpperCase()

  const response = await fetch(base + path, {
    ...init,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(safe.has(method) ? {} : { [guardHeader]: guard() }),
      ...init?.headers,
    },
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

  if (response.status === 204) return null as T

  return response.json() as Promise<T>
}

export const api = {
  me: () => request<Me>('/api/auth/me'),
  signIn: (username: string, password: string, label: string) =>
    request<Me>('/api/auth/signin', {
      method: 'POST',
      body: JSON.stringify({ username, password, label }),
    }),
  claim: (username: string, password: string, label: string) =>
    request<Me>('/api/auth/claim', {
      method: 'POST',
      body: JSON.stringify({ username, password, label }),
    }),
  signOut: () => request<void>('/api/auth/signout', { method: 'POST' }),
  changePassword: (current: string, next: string) =>
    request<Me>('/api/auth/password', {
      method: 'POST',
      body: JSON.stringify({ current, next }),
    }),

  status: () => request<ServerStatus>('/api/status'),
  devices: () => request<Device[]>('/api/devices'),
  device: (id: string) => request<Device>(`/api/devices/${encodeURIComponent(id)}`),
  heartbeat: (id: string) =>
    request<void>(`/api/devices/${encodeURIComponent(id)}/heartbeat`, { method: 'POST' }),

  currentSession: () => request<Session | null>('/api/sessions/current'),
  sessions: () => request<Session[]>('/api/sessions'),
  startSession: (subject: string, topic: string | null) =>
    request<Session>('/api/sessions', {
      method: 'POST',
      body: JSON.stringify({ subject, topic }),
    }),
  pauseSession: (id: string) => request<Session>(`/api/sessions/${id}/pause`, { method: 'POST' }),
  resumeSession: (id: string) => request<Session>(`/api/sessions/${id}/resume`, { method: 'POST' }),
  endSession: (id: string) => request<Session>(`/api/sessions/${id}/end`, { method: 'POST' }),

  goals: (date?: string) => request<Goal[]>(date ? `/api/goals?date=${date}` : '/api/goals'),
  createGoal: (subject: string, topic: string | null, targetMinutes: number, goalDate?: string) =>
    request<Goal>('/api/goals', {
      method: 'POST',
      body: JSON.stringify({
        subject,
        topic,
        target_minutes: targetMinutes,
        ...(goalDate ? { goal_date: goalDate } : {}),
      }),
    }),
  completeGoal: (id: string) => request<Goal>(`/api/goals/${id}/complete`, { method: 'POST' }),
  reopenGoal: (id: string) => request<Goal>(`/api/goals/${id}/reopen`, { method: 'POST' }),
  deleteGoal: (id: string) => request<void>(`/api/goals/${id}`, { method: 'DELETE' }),

  messages: () => request<Message[]>('/api/messages'),
  unreadMessages: () => request<Message[]>('/api/messages/unread'),
  sendMessage: (from: Sender, body: string, kind: MessageKind = 'message') =>
    request<Message>('/api/messages', {
      method: 'POST',
      body: JSON.stringify({ from, body, kind }),
    }),
  markMessageRead: (id: string) => request<Message>(`/api/messages/${id}/read`, { method: 'POST' }),
  markAllMessagesRead: () => request<{ marked: number }>('/api/messages/read', { method: 'POST' }),

  files: (category?: Shelf) =>
    request<SharedFile[]>(category ? `/api/files?category=${category}` : '/api/files'),
  deleteFile: (id: string) => request<void>(`/api/files/${id}`, { method: 'DELETE' }),
  verifyFile: (id: string) => request<IntegrityCheck>(`/api/files/${id}/verify`, { method: 'POST' }),
  downloadURL: (id: string) => `${base}/api/files/${id}/download`,

  boardSince: (since: number) => request<BoardPage>(`/api/board/marks?since=${since}`),
  drawStroke: (ink: Pen, thickness: number, path: number[]) =>
    request<Mark>('/api/board/marks', {
      method: 'POST',
      body: JSON.stringify({ ink, thickness, path }),
    }),
  eraseStroke: (id: string) => request<Mark>(`/api/board/marks/${id}`, { method: 'DELETE' }),
  clearBoard: () => request<Mark>('/api/board/clear', { method: 'POST' }),

  focus: () => request<FocusState>(`/api/focus/current?zone=${encodeURIComponent(localZone())}`),
  startFocus: (kind: FocusKind, sessionID: string | null = null) =>
    request<FocusRun>(`/api/focus?zone=${encodeURIComponent(localZone())}`, {
      method: 'POST',
      body: JSON.stringify({ kind, session_id: sessionID }),
    }),
  finishFocus: (id: string) =>
    request<FocusIdle>(`/api/focus/${id}/finish?zone=${encodeURIComponent(localZone())}`, { method: 'POST' }),
  stopFocus: (id: string) =>
    request<FocusIdle>(`/api/focus/${id}/stop?zone=${encodeURIComponent(localZone())}`, { method: 'POST' }),

  notices: () => request<NoticeList>('/api/notices'),
  markNoticesSeen: () => request<{ marked: number }>('/api/notices/seen', { method: 'POST' }),

  insights: (days: number) =>
    request<Insights>(`/api/insights?days=${days}&zone=${encodeURIComponent(localZone())}`),
}

// The browser knows its own zone; the server needs it to decide which local day a
// session belongs to.
export function localZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

// fetch cannot report how much of a request body has gone out, so the one call that
// needs a progress bar is the one call that still uses XMLHttpRequest.
export function uploadFile(
  file: File,
  category: Shelf,
  onProgress: (fraction: number) => void,
): { done: Promise<SharedFile>; abort: () => void } {
  const wire = new XMLHttpRequest()

  const done = new Promise<SharedFile>((resolve, reject) => {
    const form = new FormData()
    form.append('category', category)
    form.append('file', file)

    wire.upload.onprogress = (event) => {
      if (event.lengthComputable) onProgress(event.loaded / event.total)
    }

    wire.onload = () => {
      if (wire.status >= 200 && wire.status < 300) {
        resolve(JSON.parse(wire.responseText) as SharedFile)
        return
      }

      let message = `upload failed with ${wire.status}`
      try {
        const body = JSON.parse(wire.responseText)
        if (body?.error) message = body.error
      } catch {
        // the server did not send our error shape; the status is all we have
      }
      reject(new RequestFailed(wire.status, message))
    }

    wire.onerror = () => reject(new RequestFailed(0, 'the server could not be reached'))
    wire.onabort = () => reject(new RequestFailed(0, 'upload cancelled'))

    wire.open('POST', base + '/api/files')
    wire.withCredentials = true
    wire.setRequestHeader(guardHeader, guard())
    wire.send(form)
  })

  return { done, abort: () => wire.abort() }
}
