package deliveryapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
)

// authenticator validates Entra ID tokens with the Nais Texas introspection endpoint. Which
// applications can obtain a token is decided by the Nais access policy.
type authenticator struct {
	endpoint string
	client   *http.Client
	log      logrus.FieldLogger
}

type introspectionResult struct {
	Active bool `json:"active"`
}

func (a *authenticator) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			unauthorized(w)
			return
		}
		res, err := a.introspect(r.Context(), token)
		if err != nil {
			a.log.WithError(err).Errorf("token introspection failed")
			writeProblem(w, newProblem(problemUpstreamError, "token validation is unavailable"))
			return
		}
		if !res.Active {
			a.log.Warnf("rejected inactive token")
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *authenticator) introspect(ctx context.Context, token string) (*introspectionResult, error) {
	body, err := json.Marshal(map[string]string{"identity_provider": "azuread", "token": token})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspection returned HTTP %d", resp.StatusCode)
	}
	var res introspectionResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode introspection response: %w", err)
	}
	return &res, nil
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
}
