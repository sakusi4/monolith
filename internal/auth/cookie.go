package auth

import (
	"net"
	"net/http"
)

const cookieName = "session"

func sessionCookie(r *http.Request, token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   !isLocalhost(r.Host),
		SameSite: http.SameSiteLaxMode,
	}
}

func isLocalhost(host string) bool {
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	return name == "localhost" || net.ParseIP(name).IsLoopback()
}
