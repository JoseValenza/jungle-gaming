
package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

type providerContextKey struct{}

type tokenClaims struct {
	ProviderID string `json:"preferred_username"`
	AuthorizedParty string `json:"azp"`
}

func NewOIDCAuth() (func(http.Handler) http.Handler, error) {
	issuerURL := strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL"))
	if issuerURL == "" {
		return nil, fmt.Errorf("OIDC_ISSUER_URL não configurado")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	client := &http.Client{
		Transport: &issuerTransport{
			base: http.DefaultTransport,
		},
	}

	ctx = oidc.ClientContext(ctx, client)

	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("inicializar Keycloak: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{
		SkipClientIDCheck: true,
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			parts := strings.SplitN(authHeader, " ", 2)

			if len(parts) != 2 ||
				!strings.EqualFold(parts[0], "Bearer") ||
				strings.TrimSpace(parts[1]) == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "token Bearer obrigatório",
				})
				return
			}

			token, err := verifier.Verify(r.Context(), parts[1])
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "token inválido",
				})
				return
			}

			var claims tokenClaims
			if err := token.Claims(&claims); err != nil ||
				claims.AuthorizedParty != "jungle-api" ||
				strings.TrimSpace(claims.ProviderID) == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "identidade do provedor inválida",
				})
				return
			}

			ctx := context.WithValue(
				r.Context(),
				providerContextKey{},
				claims.ProviderID,
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}, nil
}

type issuerTransport struct {
	base http.RoundTripper
}

func (t *issuerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	request := r.Clone(r.Context())

	if request.URL.Host == "localhost:8081" {
		request.Host = request.URL.Host
		request.URL.Host = "host.docker.internal:8081"
	}

	return t.base.RoundTrip(request)
}

func authenticatedProviderID(r *http.Request) string {
	providerID, _ := r.Context().Value(providerContextKey{}).(string)
	return providerID
}
