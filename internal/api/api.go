package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"udevs/face_control/internal/device"
	"udevs/face_control/internal/registration"
	"udevs/face_control/internal/storage"
	"udevs/face_control/internal/user"
)

type Handler struct {
	Devices       *device.Repo
	Users         *user.Repo
	Registrations *registration.Service
	Photos        storage.PhotoStore
	Log           *slog.Logger
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/devices", func(r chi.Router) {
		r.Post("/", h.createDevice)
		r.Get("/", h.listDevices)
		r.Get("/{id}", h.getDevice)
		r.Post("/{device_id}/registrations", h.registerUserOnDevice)
		r.Delete("/{device_id}/registrations/{user_id}", h.deleteRegistration)
	})

	r.Post("/users", h.createUser)
	return r
}

// ---------- devices ----------

type createDeviceReq struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	MAC      string `json:"mac,omitempty"`
}

func (h *Handler) createDevice(w http.ResponseWriter, r *http.Request) {
	var req createDeviceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Name == "" || req.IP == "" {
		writeError(w, http.StatusBadRequest, "name and ip are required")
		return
	}
	d := &device.Device{
		Name:     req.Name,
		IP:       req.IP,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
	}
	if req.MAC != "" {
		d.MAC = &req.MAC
	}
	if err := h.Devices.Create(r.Context(), d); err != nil {
		h.Log.Error("create device", "err", err)
		writeError(w, http.StatusInternalServerError, "create device failed")
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	devs, err := h.Devices.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, devs)
}

func (h *Handler) getDevice(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	d, err := h.Devices.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// ---------- users ----------

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB matches SDK limit
		writeError(w, http.StatusBadRequest, "multipart parse failed")
		return
	}
	fullName := r.FormValue("full_name")
	if fullName == "" {
		writeError(w, http.StatusBadRequest, "full_name required")
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "photo file required")
		return
	}
	defer file.Close()

	key, err := h.Photos.Save(r.Context(), file, header)
	if err != nil {
		h.Log.Error("save photo", "err", err)
		writeError(w, http.StatusInternalServerError, "photo save failed")
		return
	}
	u := &user.User{FullName: fullName, PhotoKey: key}
	if err := h.Users.Create(r.Context(), u); err != nil {
		h.Log.Error("create user", "err", err)
		writeError(w, http.StatusInternalServerError, "create user failed")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

// ---------- registrations ----------

type registerReq struct {
	UserID string `json:"user_id"`
}

func (h *Handler) registerUserOnDevice(w http.ResponseWriter, r *http.Request) {
	deviceID, err := uuid.Parse(chi.URLParam(r, "device_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device_id")
		return
	}
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	reg, err := h.Registrations.RegisterUser(r.Context(), deviceID, userID)
	if err != nil {
		switch {
		case errors.Is(err, registration.ErrDeviceNotFound):
			writeError(w, http.StatusNotFound, "device not found")
		case errors.Is(err, registration.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "user not found")
		default:
			h.Log.Error("register", "err", err)
			// reg may carry the failed row; still return it so the caller sees status=failed
			if reg != nil {
				writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "registration": reg})
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, reg)
}

func (h *Handler) deleteRegistration(w http.ResponseWriter, r *http.Request) {
	deviceID, err := uuid.Parse(chi.URLParam(r, "device_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device_id")
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	if err := h.Registrations.DeleteRegistration(r.Context(), deviceID, userID); err != nil {
		if errors.Is(err, registration.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "device not found")
			return
		}
		h.Log.Error("delete registration", "err", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
