// Package httpapi is the HTTP surface of the Deskbridge server.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

// DeviceStore is declared here, where it is used, so handlers can be tested with a fake.
type DeviceStore interface {
	RegisterDevice(ctx context.Context, device store.Device) (store.Device, error)
	Device(ctx context.Context, id string) (store.Device, error)
	Devices(ctx context.Context) ([]store.Device, error)
	RecordHeartbeat(ctx context.Context, id string) error
}

type Server struct {
	log           *slog.Logger
	version       string
	startedAt     time.Time
	devices       DeviceStore
	allowedOrigin string
}

func NewServer(log *slog.Logger, version string, startedAt time.Time, devices DeviceStore, allowedOrigin string) *Server {
	return &Server{
		log:           log,
		version:       version,
		startedAt:     startedAt,
		devices:       devices,
		allowedOrigin: allowedOrigin,
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

	return s.withRequestLogging(s.withCORS(mux))
}
