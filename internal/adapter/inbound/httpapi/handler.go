package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/siravitdell/user-management-api/internal/core/port"
)

type Handler struct {
	users  port.UserService
	auth   port.AuthService
	logger *slog.Logger
	now    func() time.Time
}

func NewRouter(users port.UserService, auth port.AuthService, logger *slog.Logger) http.Handler {
	h := &Handler{users: users, auth: auth, logger: logger, now: time.Now}
	protected := RequireAuth(auth)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register", h.createUser)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)

	mux.Handle("POST /api/v1/users", protected(http.HandlerFunc(h.createUser)))
	mux.Handle("GET /api/v1/users", protected(http.HandlerFunc(h.listUsers)))
	mux.Handle("GET /api/v1/users/{id}", protected(http.HandlerFunc(h.getUser)))
	mux.Handle("PATCH /api/v1/users/{id}", protected(http.HandlerFunc(h.updateUser)))
	mux.Handle("DELETE /api/v1/users/{id}", protected(http.HandlerFunc(h.deleteUser)))

	return Logging(logger)(Recoverer(logger)(mux))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	token, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeServiceError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toTokenResponse(token, h.now()))
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	user, err := h.users.Create(r.Context(), port.CreateUserInput(req))
	if err != nil {
		writeServiceError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUserResponse(user))
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	user, err := h.users.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		writeServiceError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponses(users))
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var req updateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	user, err := h.users.Update(r.Context(), r.PathValue("id"), port.UpdateUserInput(req))
	if err != nil {
		writeServiceError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.users.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeServiceError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
