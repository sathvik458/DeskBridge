# Deskbridge

A bridge between two study desks, about 2,500 km apart.

I built this so I can help a family member study from another country. An old
Windows PC in Bahrain runs the Deskbridge server. A student laptop and an old
phone, used as a desk camera, sit on the same local network. I connect from my
Mac in India and can see whether the setup is online, start and track study
sessions, set daily goals, send messages and share files.

This is a personal distributed system, not a product. It is built phase by
phase, and the point is that I can explain every engineering decision in it.

## How it works

One authoritative Go server owns all state. Every other device is a thin
client that asks the server. The design splits into three planes:

- **Control plane** - the REST API, backed by SQLite. Source of truth.
- **Real-time plane** - a WebSocket event stream for timers, messages and
  device status. Deliberately disposable: if it drops, clients re-fetch over
  REST and nothing is lost.
- **Media plane** - the desk camera, kept separate so a camera failure cannot
  take down sessions or messaging.

That last property is the point. The server runs on old, unreliable hardware,
so the system is designed to be restarted at any moment without losing state.

## Status

Early. Currently building the Go server foundation.

## Layout

```
backend/    Go server - API, database, sessions, files
```

More directories get added as their phase begins.

## Running the server

Requires Go 1.22 or newer.

```
cd backend
go run ./cmd/deskbridge-server
```

Then:

```
curl http://localhost:8080/health
```

## Tech

Go for the server, SQLite for storage, React for the dashboard. Python is used
only for camera work, and C++ only where a systems-level reason exists.

## License

MIT
