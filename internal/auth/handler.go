package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/sakusi4/monolith/web"
)

type handler struct {
	store *Store
}

type loginPage struct {
	Email string
	Error string
}

func NewHandler(store *Store) http.Handler {
	h := &handler{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/login", h.loginForm)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/logout", h.logout)
	return mux
}

func (h *handler) loginForm(w http.ResponseWriter, r *http.Request) {
	web.Render(w, r, http.StatusOK, "login", loginPage{})
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	email := r.PostFormValue("email")
	token, err := h.store.Login(r.Context(), email, r.PostFormValue("password"))
	if errors.Is(err, ErrInvalidCredentials) {
		web.Render(w, r, http.StatusUnauthorized, "login", loginPage{
			Email: email,
			Error: "Incorrect email or password.",
		})
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	http.SetCookie(w, sessionCookie(r, token, int(sessionTTL/time.Second)))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		if err := h.store.Logout(r.Context(), cookie.Value); err != nil {
			web.ServerError(w, r, err)
			return
		}
	}
	http.SetCookie(w, sessionCookie(r, "", -1))
	http.Redirect(w, r, loginURL, http.StatusSeeOther)
}
