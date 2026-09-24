package auth

import (
	"errors"
	"net/http"

	"github.com/sakusi4/monolith/web"
)

const loginURL = "/auth/login"

func Require(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil {
				http.Redirect(w, r, loginURL, http.StatusSeeOther)
				return
			}
			_, err = store.UserBySession(r.Context(), cookie.Value)
			if errors.Is(err, ErrNoSession) {
				http.Redirect(w, r, loginURL, http.StatusSeeOther)
				return
			}
			if err != nil {
				web.ServerError(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
