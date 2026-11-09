package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

var deviceKinds = []string{"server", "laptop", "phone", "desktop"}

// user_id is still accepted so older clients keep working, but it is never used:
// a device belongs to whoever is signed in when it registers.
type registerDeviceRequest struct {
	ID     string  `json:"id"`
	UserID *string `json:"user_id"`
	Name   string  `json:"name"`
	Kind   string  `json:"kind"`
}

func (req registerDeviceRequest) validate() error {
	if strings.TrimSpace(req.ID) == "" {
		return errors.New("id is required")
	}

	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}

	for _, kind := range deviceKinds {
		if req.Kind == kind {
			return nil
		}
	}

	return fmt.Errorf("kind must be one of %s", strings.Join(deviceKinds, ", "))
}

type deviceResponse struct {
	ID         string  `json:"id"`
	UserID     *string `json:"user_id"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	Status     string  `json:"status"`
	LastSeenAt *string `json:"last_seen_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

func newDeviceResponse(device store.Device) deviceResponse {
	response := deviceResponse{
		ID:        device.ID,
		UserID:    device.UserID,
		Name:      device.Name,
		Kind:      device.Kind,
		Status:    device.Status,
		CreatedAt: device.CreatedAt.Format(time.RFC3339),
		UpdatedAt: device.UpdatedAt.Format(time.RFC3339),
	}

	if device.LastSeenAt != nil {
		lastSeen := device.LastSeenAt.Format(time.RFC3339)
		response.LastSeenAt = &lastSeen
	}

	return response
}

func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	var req registerDeviceRequest

	if err := decodeBody(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := req.validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	me := userOf(r)
	id := strings.TrimSpace(req.ID)

	// Registering is an upsert keyed on an id the client picks, so without this
	// check anyone signed in could rename or take over the other person's device.
	known, err := s.devices.Device(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		s.log.Error("looking up device", "device_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not register device")
		return
	case known.UserID != nil && *known.UserID != me.ID:
		s.writeError(w, http.StatusForbidden, "that device belongs to someone else")
		return
	}

	device, err := s.devices.RegisterDevice(r.Context(), store.Device{
		ID:     id,
		UserID: &me.ID,
		Name:   strings.TrimSpace(req.Name),
		Kind:   req.Kind,
	})
	if err != nil {
		s.log.Error("registering device", "device_id", req.ID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not register device")
		return
	}

	s.tell(r, live.DeviceChanged, map[string]any{"device_id": device.ID, "status": device.Status})

	s.writeJSON(w, http.StatusOK, newDeviceResponse(device))
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.devices.Devices(r.Context())
	if err != nil {
		s.log.Error("listing devices", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not list devices")
		return
	}

	responses := make([]deviceResponse, 0, len(devices))
	for _, device := range devices {
		responses = append(responses, newDeviceResponse(device))
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func (s *Server) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	device, err := s.devices.Device(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		s.log.Error("looking up device", "device_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not look up device")
		return
	}

	s.writeJSON(w, http.StatusOK, newDeviceResponse(device))
}

func (s *Server) handleDeviceHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := s.devices.RecordHeartbeat(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		s.log.Error("recording heartbeat", "device_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not record heartbeat")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
