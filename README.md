# Deskbridge

A bridge between two study desks, about 2,500 km apart.

Deskbridge is a personal distributed system I’m building to help a family member study from another country. An old Windows PC in Bahrain acts as the server, while a student laptop and an old phone handle the client and camera side. I connect from my Mac in India to manage and keep track of the setup remotely.

The project is being built phase by phase, with an emphasis on understanding and owning every engineering decision behind it.

## How it works

One Go server acts as the source of truth, with the system split into a few deliberately separate parts:

- **Control plane** - REST API and SQLite for persistent state.
- **Real-time plane** - WebSockets for events such as timers, messages, and device status.
- **Media plane** - Camera functionality kept separate from the core system.

The server is expected to run on old hardware, so reliability and recoverability are part of the design rather than afterthoughts.

## Status

**In the works.**

Currently building the Go server foundation.

## Layout

```text
backend/    Go server

More will appear as each phase takes shape.

Tech

Go, SQLite, React, with Python and C++ introduced only where they have a reason to exist.