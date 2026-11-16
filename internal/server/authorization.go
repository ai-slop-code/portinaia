package server

import (
	"net/http"
)

type credentialClass string

const (
	credentialHumanSession credentialClass = "human_session"
	credentialAgent        credentialClass = "agent"
	credentialEnrollment   credentialClass = "enrollment"
)

// Principal describes the credential class authenticated for a request. Credentials
// are never upgraded or downgraded between classes.
type Principal struct {
	Class  credentialClass
	ID     string
	Scopes map[string]struct{}
}

func (p Principal) HasCapability(capability string) bool {
	if _, ok := p.Scopes[capability]; ok {
		return true
	}
	return false
}

// SoleAdministratorPrincipal is the explicit capability grant for the sole enabled
// browser administrator; the role name itself is never treated as a wildcard.
func SoleAdministratorPrincipal(id string) Principal {
	return Principal{Class: credentialHumanSession, ID: id, Scopes: map[string]struct{}{
		"admin": {}, "inventory:read": {}, "inventory:write": {}, "domains:read": {},
		"domains:write": {}, "audit:read": {}, "discovery:read": {}, "discovery:write": {},
	}}
}

func (p Principal) Require(capability string) bool { return p.HasCapability(capability) }

func rejectCredentialClass(r *http.Request, allowed credentialClass) bool {
	if r.Header.Get("Authorization") != "" && allowed != credentialAgent {
		return true
	}
	if _, err := r.Cookie(sessionCookie); err == nil && allowed != credentialHumanSession {
		return true
	}
	return false
}
