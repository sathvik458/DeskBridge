// Package httpapi is the HTTP surface of the Deskbridge server.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
	"github.com/sathvik458/deskbridge/backend/internal/vault"
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

type MessageStore interface {
	CreateMessage(ctx context.Context, message store.Message) (store.Message, error)
	Messages(ctx context.Context, limit int) ([]store.Message, error)
	UnreadNotFrom(ctx context.Context, viewerID string) ([]store.Message, error)
	MarkMessageRead(ctx context.Context, id string) (store.Message, error)
	MarkAllReadNotFrom(ctx context.Context, viewerID string) (int, error)
}

type FileStore interface {
	RecordUpload(ctx context.Context, file store.File) (store.File, error)
	File(ctx context.Context, id string) (store.File, error)
	Files(ctx context.Context, category string) ([]store.File, error)
	ForgetFile(ctx context.Context, id string) (store.File, error)
}

type UserStore interface {
	User(ctx context.Context, id string) (store.User, error)
	UserByUsername(ctx context.Context, username string) (store.User, error)
	OneStudent(ctx context.Context) (store.User, error)
	PassHash(ctx context.Context, id string) (string, error)
	ClaimPassword(ctx context.Context, id, hash string) (bool, error)
}

type LoginStore interface {
	StartLogin(ctx context.Context, l store.Login) (store.Login, error)
	WhoIs(ctx context.Context, token string) (store.User, store.Login, error)
	TouchLogin(ctx context.Context, token string, stale time.Duration) error
	EndLogin(ctx context.Context, token string) error
	SweepLogins(ctx context.Context) (int, error)
}

type BoardStore interface {
	AddMark(ctx context.Context, mark store.Mark) (store.Mark, error)
	MarksSince(ctx context.Context, since int64, limit int) ([]store.Mark, error)
	MarkExists(ctx context.Context, id string) (bool, error)
	TrimBoard(ctx context.Context) (int, error)
}

type sessionMove func(ctx context.Context, id string) (store.Session, error)

type Server struct {
	log           *slog.Logger
	version       string
	startedAt     time.Time
	devices       DeviceStore
	sessions      SessionStore
	goals         GoalStore
	messages      MessageStore
	files         FileStore
	board         BoardStore
	users         UserStore
	logins        LoginStore
	vault         *vault.Vault
	secureCookies bool
	feed          *live.Feed
	allowedOrigin string
	now           func() time.Time
}

func NewServer(log *slog.Logger, version string, startedAt time.Time, devices DeviceStore, sessions SessionStore, goals GoalStore, messages MessageStore, files FileStore, board BoardStore, users UserStore, logins LoginStore, vault *vault.Vault, feed *live.Feed, allowedOrigin string, secureCookies bool) *Server {
	return &Server{
		log:           log,
		version:       version,
		startedAt:     startedAt,
		devices:       devices,
		sessions:      sessions,
		goals:         goals,
		messages:      messages,
		files:         files,
		board:         board,
		users:         users,
		logins:        logins,
		vault:         vault,
		secureCookies: secureCookies,
		feed:          feed,
		allowedOrigin: allowedOrigin,
		now:           func() time.Time { return time.Now().UTC() },
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/status", s.handleStatus)

	mux.HandleFunc("POST /api/auth/claim", s.handleClaim)
	mux.HandleFunc("POST /api/auth/signin", s.handleSignIn)
	mux.HandleFunc("POST /api/auth/signout", s.handleSignOut)

	gated := http.NewServeMux()

	mux.Handle("/api/", s.withSignIn(gated))

	gated.HandleFunc("GET /api/auth/me", s.handleMe)

	gated.HandleFunc("POST /api/devices/register", s.handleRegisterDevice)
	gated.HandleFunc("GET /api/devices", s.handleListDevices)
	gated.HandleFunc("GET /api/devices/{id}", s.handleGetDevice)
	gated.HandleFunc("POST /api/devices/{id}/heartbeat", s.handleDeviceHeartbeat)

	gated.HandleFunc("POST /api/sessions", s.handleStartSession)
	gated.HandleFunc("GET /api/sessions", s.handleListSessions)
	gated.HandleFunc("GET /api/sessions/current", s.handleCurrentSession)
	gated.HandleFunc("POST /api/sessions/{id}/pause", s.handlePauseSession)
	gated.HandleFunc("POST /api/sessions/{id}/resume", s.handleResumeSession)
	gated.HandleFunc("POST /api/sessions/{id}/end", s.handleEndSession)

	gated.HandleFunc("GET /api/goals", s.handleListGoals)
	gated.HandleFunc("POST /api/goals", s.handleCreateGoal)
	gated.HandleFunc("PATCH /api/goals/{id}", s.handleUpdateGoal)
	gated.HandleFunc("POST /api/goals/{id}/complete", s.handleCompleteGoal)
	gated.HandleFunc("POST /api/goals/{id}/reopen", s.handleReopenGoal)
	gated.HandleFunc("DELETE /api/goals/{id}", s.handleDeleteGoal)

	gated.HandleFunc("GET /api/messages", s.handleListMessages)
	gated.HandleFunc("GET /api/messages/unread", s.handleUnreadMessages)
	gated.HandleFunc("POST /api/messages", s.handleCreateMessage)
	gated.HandleFunc("POST /api/messages/read", s.handleMarkAllRead)
	gated.HandleFunc("POST /api/messages/{id}/read", s.handleMarkMessageRead)

	gated.HandleFunc("GET /api/files", s.handleListFiles)
	gated.HandleFunc("POST /api/files", s.handleUploadFile)
	gated.HandleFunc("GET /api/files/{id}/download", s.handleDownloadFile)
	gated.HandleFunc("POST /api/files/{id}/verify", s.handleVerifyFile)
	gated.HandleFunc("DELETE /api/files/{id}", s.handleDeleteFile)

	gated.HandleFunc("GET /api/board/marks", s.handleBoardMarks)
	gated.HandleFunc("POST /api/board/marks", s.handleDrawStroke)
	gated.HandleFunc("DELETE /api/board/marks/{id}", s.handleEraseStroke)
	gated.HandleFunc("POST /api/board/clear", s.handleClearBoard)

	gated.HandleFunc("GET /api/live", s.handleLiveStream)

	return s.withRequestLogging(s.withCORS(mux))
}
