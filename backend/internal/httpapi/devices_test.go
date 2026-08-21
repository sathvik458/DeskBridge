package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

type fakeDeviceStore struct {
	devices      map[string]store.Device
	registered   store.Device
	heartbeatFor string
	err          error
}

func newFakeDeviceStore() *fakeDeviceStore {
	return &fakeDeviceStore{devices: map[string]store.Device{}}
}

func (f *fakeDeviceStore) RegisterDevice(ctx context.Context, device store.Device) (store.Device, error) {
	if f.err != nil {
		return store.Device{}, f.err
	}

	now := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	device.Status = store.StatusOnline
	device.LastSeenAt = &now
	device.CreatedAt = now
	device.UpdatedAt = now

	f.registered = device
	f.devices[device.ID] = device

	return device, nil
}

func (f *fakeDeviceStore) Device(ctx context.Context, id string) (store.Device, error) {
	if f.err != nil {
		return store.Device{}, f.err
	}

	device, ok := f.devices[id]
	if !ok {
		return store.Device{}, store.ErrNotFound
	}

	return device, nil
}

func (f *fakeDeviceStore) Devices(ctx context.Context) ([]store.Device, error) {
	if f.err != nil {
		return nil, f.err
	}

	devices := []store.Device{}
	for _, device := range f.devices {
		devices = append(devices, device)
	}

	return devices, nil
}

func (f *fakeDeviceStore) RecordHeartbeat(ctx context.Context, id string) error {
	if f.err != nil {
		return f.err
	}

	if _, ok := f.devices[id]; !ok {
		return store.ErrNotFound
	}

	f.heartbeatFor = id

	return nil
}

func post(t *testing.T, s *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	return rec
}

func TestRegisterDevice(t *testing.T) {
	devices := newFakeDeviceStore()
	s := newTestServerWithStore(time.Now(), devices)

	rec := post(t, s, "/api/devices/register",
		`{"id":"laptop-1","name":"Student Laptop","kind":"laptop"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body)
	}

	var body deviceResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if body.ID != "laptop-1" || body.Name != "Student Laptop" {
		t.Errorf("response = %+v, want the registered device", body)
	}

	if body.Status != store.StatusOnline {
		t.Errorf("Status = %q, want %q", body.Status, store.StatusOnline)
	}

	if body.LastSeenAt == nil {
		t.Error("last_seen_at is null, want the registration time")
	}
}

func TestRegisterDeviceTrimsWhitespace(t *testing.T) {
	devices := newFakeDeviceStore()
	s := newTestServerWithStore(time.Now(), devices)

	post(t, s, "/api/devices/register", `{"id":"  laptop-1  ","name":"  Laptop  ","kind":"laptop"}`)

	if devices.registered.ID != "laptop-1" {
		t.Errorf("stored ID = %q, want it trimmed to %q", devices.registered.ID, "laptop-1")
	}
	if devices.registered.Name != "Laptop" {
		t.Errorf("stored Name = %q, want it trimmed to %q", devices.registered.Name, "Laptop")
	}
}

func TestRegisterDeviceRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing id", body: `{"name":"Laptop","kind":"laptop"}`},
		{name: "blank id", body: `{"id":"   ","name":"Laptop","kind":"laptop"}`},
		{name: "missing name", body: `{"id":"d1","kind":"laptop"}`},
		{name: "unknown kind", body: `{"id":"d1","name":"Laptop","kind":"toaster"}`},
		{name: "missing kind", body: `{"id":"d1","name":"Laptop"}`},
		{name: "not json", body: `hello`},
		{name: "unknown field", body: `{"id":"d1","name":"Laptop","kind":"laptop","admin":true}`},
		{name: "two objects", body: `{"id":"d1","name":"L","kind":"laptop"}{"id":"d2"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServerWithStore(time.Now(), newFakeDeviceStore())

			rec := post(t, s, "/api/devices/register", tc.body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

func TestListDevicesReturnsEmptyArrayNotNull(t *testing.T) {
	s := newTestServerWithStore(time.Now(), newFakeDeviceStore())

	rec := doRequest(t, s, http.MethodGet, "/api/devices")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("body = %s, want []", got)
	}
}

func TestGetDeviceReturns404WhenMissing(t *testing.T) {
	s := newTestServerWithStore(time.Now(), newFakeDeviceStore())

	rec := doRequest(t, s, http.MethodGet, "/api/devices/ghost")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding error response: %v", err)
	}

	if body.Error == "" {
		t.Error("error message is empty")
	}
}

func TestGetDeviceReturnsTheDevice(t *testing.T) {
	devices := newFakeDeviceStore()
	s := newTestServerWithStore(time.Now(), devices)

	post(t, s, "/api/devices/register", `{"id":"phone-1","name":"Phone Camera","kind":"phone"}`)

	rec := doRequest(t, s, http.MethodGet, "/api/devices/phone-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body deviceResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if body.ID != "phone-1" {
		t.Errorf("ID = %q, want %q", body.ID, "phone-1")
	}
}

func TestHeartbeatReturns204(t *testing.T) {
	devices := newFakeDeviceStore()
	s := newTestServerWithStore(time.Now(), devices)

	post(t, s, "/api/devices/register", `{"id":"laptop-1","name":"Laptop","kind":"laptop"}`)

	rec := post(t, s, "/api/devices/laptop-1/heartbeat", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusNoContent, rec.Body)
	}

	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want it empty for 204", rec.Body)
	}

	if devices.heartbeatFor != "laptop-1" {
		t.Errorf("heartbeat recorded for %q, want %q", devices.heartbeatFor, "laptop-1")
	}
}

func TestHeartbeatReturns404ForUnknownDevice(t *testing.T) {
	s := newTestServerWithStore(time.Now(), newFakeDeviceStore())

	rec := post(t, s, "/api/devices/ghost/heartbeat", "")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStoreFailureBecomes500(t *testing.T) {
	devices := newFakeDeviceStore()
	devices.err = context.DeadlineExceeded
	s := newTestServerWithStore(time.Now(), devices)

	rec := doRequest(t, s, http.MethodGet, "/api/devices")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	if strings.Contains(rec.Body.String(), "deadline") {
		t.Errorf("internal error leaked to the client: %s", rec.Body)
	}
}
