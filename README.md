# Deskbridge

**Be there for a student, from anywhere.**

Deskbridge is a private study space you run on your own hardware. See the desk, keep
time, set goals, answer questions, and work through a problem together — without
either person leaving home.

It is built for anyone supporting a student at a distance:

- **Parents working away from home** — stay part of your child's study routine across
  a time zone.
- **Relatives helping out** — an aunt, uncle or older sibling who is good at maths and
  lives somewhere else.
- **Tuition teachers with remote students** — run structured sessions, set homework,
  track hours, and keep a record per student.

## What it does

| | |
|---|---|
| **Desk view** | An old phone becomes the study-desk camera |
| **Study sessions** | Start, pause, resume and end — timed by the server, not the browser |
| **Daily goals** | Subject, topic, target minutes, and progress through the day |
| **Messages** | Ordinary messages, and help requests the student raises themselves |
| **Shared whiteboard** | Draw a diagram at one end, it appears at the other |
| **Files** | Worksheets, notes, photographs of homework |
| **Focus mode** | Pomodoro cycles kept in sync on both screens |
| **Statistics** | Hours by subject, by day, by week |

## What it is not

Deskbridge is **not monitoring software**. There is no attention scoring, no face or
emotion detection, no hidden recording, and no report on a person's behaviour. The
camera shows a desk, and it says so loudly whenever it is on. Help is *asked for* by
the student, never extracted.

The design rule: both people should be comfortable with every screen. If a screen
would feel uncomfortable to the student looking over the supporter's shoulder, it is
wrong.

## Yours, on your own hardware

Deskbridge runs on a computer you already own — an old laptop or desktop is plenty.
Nothing is stored on anyone else's servers. Study records, photographs and messages
stay on your machine, and the two ends connect over a private network rather than the
open internet.

Runs on Windows, macOS and Linux.

## How it works

One authoritative Go server owns all state. Every other device is a thin client that
asks the server. The system splits into three planes:

- **Control plane** — a REST API backed by SQLite. The source of truth.
- **Real-time plane** — a WebSocket event stream for timers, messages and device
  status. Deliberately disposable: if it drops, clients re-fetch over REST and nothing
  is lost.
- **Media plane** — the camera, kept separate so a camera failure cannot take down
  sessions or messaging.

The server is expected to run on old, unreliable hardware, so restart and recovery are
part of the design rather than an afterthought. A study session survives a reboot
mid-session; devices are presumed offline after a heartbeat silence rather than
assumed present.

## Stack

Go and SQLite on the server, React on the dashboard. Python is used only for camera
work, and C++ only where a systems-level reason exists. Dependencies are kept
deliberately few — the server has exactly one.

## Status

In active development, built phase by phase.

**Working:** HTTP server with graceful shutdown, configuration, structured logging,
SQLite with migrations, device registration and heartbeats, a background presence
worker, and the data layer for users and devices.

**Next:** the dashboard, study sessions, goals, and the real-time event stream.

## Running the server

Requires Go 1.22 or newer.

```
cd backend
go run ./cmd/deskbridge-server
```

Then:

```
curl http://localhost:8080/health
curl http://localhost:8080/api/status
```

Configuration is read from the environment:

| Variable | Default | Meaning |
|---|---|---|
| `DESKBRIDGE_ADDR` | `localhost:8080` | Address the server binds to |
| `DESKBRIDGE_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `DESKBRIDGE_DB_PATH` | `deskbridge.db` | SQLite file |
| `DESKBRIDGE_SHUTDOWN_GRACE` | `10s` | How long in-flight requests may finish |
| `DESKBRIDGE_BUSY_TIMEOUT` | `5s` | How long to wait for a database write lock |
| `DESKBRIDGE_DEVICE_TIMEOUT` | `90s` | Silence after which a device is presumed offline |
| `DESKBRIDGE_SWEEP_INTERVAL` | `30s` | How often presence is checked |

## Tests

```
cd backend
go test ./...
go test -race ./...
```

## Layout

```
backend/     Go server — API, database, devices, background workers
frontend/    React dashboard
```

More directories arrive as their phase begins.

## License

MIT
