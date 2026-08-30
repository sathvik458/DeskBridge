// Package httpapi is the HTTP surface of the Deskbridge server.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

// Declared here, where they are used, so handlers can be tested with fakes.
type DeviceStore interface {
	RegisterDevice(ctx context.Context, device store.Device) (store.Device, error)
	Device(ctx context.Context, id string) (store.Device, error)
	Devices(ctx context.Context) ([]store.Device, error)
	RecordHeartbeat(ctx context.Context, id string) error
}

type SessionStore interface {
	StartSession(ctx context.Context, session store.Session) (store.Session, error)
	CurrentSession(ctx context.Context, userID string) (store.Session, error)
	Sessions(ctx context.Context, userID string, limit int) ([]store.Session, error)
	PauseSession(ctx context.Context, id string) (store.Session, error)
	ResumeSession(ctx context.Context, id string) (store.Session, error)
	EndSession(ctx context.Context, id string) (store.Session, error)
}

type GoalStore interface {
	CreateGoal(ctx context.Context, goal store.Goal) (store.Goal, error)
	Goal(ctx context.Context, id string) (store.Goal, error)
	GoalsOn(ctx context.Context, userID, date string) ([]store.Goal, error)
	UpdateGoal(ctx context.Context, goal store.Goal) (store.Goal, error)
	SetGoalDone(ctx context.Context, id string, done bool) (store.Goal, error)
	DeleteGoal(ctx context.Context, id string) error
}

type sessionMove func(ctx context.Context, id string) (store.Session, error)

type Server struct {
	log           *slog.Logger
	version       string
	startedAt     time.Time
	devices       DeviceStore
	sessions      SessionStore
	goals         GoalStore
	allowedOrigin string
	now           func() time.Time
}

func NewServer(log *slog.Logger, version string, startedAt time.Time, devices DeviceStore, sessions SessionStore, goals GoalStore, allowedOrigin string) *Server {
	return &Server{
		log:           log,
		version:       version,
		startedAt:     startedAt,
		devices:       devices,
		sessions:      sessions,
		goals:         goals,
		allowedOrigin: allowedOrigin,
		now:           func() time.Time { return time.Now().UTC() },
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/status", s.handleStatus)

	mux.HandleFunc("POST /api/devices/register", s.handleRegisterDevice)
	mux.HandleFunc("GET /api/devices", s.handleListDevices)
	mux.HandleFunc("GET /api/devices/{id}", s.handleGetDevice)
	mux.HandleFunc("POST /api/devices/{id}/heartbeat", s.handleDeviceHeartbeat)

	mux.HandleFunc("POST /api/sessions", s.handleStartSession)
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("GET /api/sessions/current", s.handleCurrentSession)
	mux.HandleFunc("POST /api/sessions/{id}/pause", s.handlePauseSession)
	mux.HandleFunc("POST /api/sessions/{id}/resume", s.handleResumeSession)
	mux.HandleFunc("POST /api/sessions/{id}/end", s.handleEndSession)

	mux.HandleFunc("GET /api/goals", s.handleListGoals)
	mux.HandleFunc("POST /api/goals", s.handleCreateGoal)
	mux.HandleFunc("PATCH /api/goals/{id}", s.handleUpdateGoal)
	mux.HandleFunc("POST /api/goals/{id}/complete", s.handleCompleteGoal)
	mux.HandleFunc("POST /api/goals/{id}/reopen", s.handleReopenGoal)
	mux.HandleFunc("DELETE /api/goals/{id}", s.handleDeleteGoal)

	return s.withRequestLogging(s.withCORS(mux))
}
