// The forms service: the public face of hosted lead-capture forms. Serves
// /f/<publicID> pages, the /forms.js embed loader and public submissions on
// its own port, so form traffic never touches the API origin. No Postgres,
// no Redis, no event bus: the backend's internal API (BACKEND_INTERNAL_URL +
// INTERNAL_API_TOKEN, the same pair the tracking service uses) is its only
// dependency.
package main

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/warmbly/warmbly/internal/formserver"
)

func main() {
	backendURL := strings.TrimSpace(os.Getenv("BACKEND_INTERNAL_URL"))
	if backendURL == "" {
		log.Fatal("BACKEND_INTERNAL_URL is required (the backend's internal API base, e.g. http://localhost:8080)")
	}
	token := os.Getenv("INTERNAL_API_TOKEN")
	if token == "" {
		log.Fatal("INTERNAL_API_TOKEN is required (must match the backend's)")
	}

	submitLimit := 0
	if v := os.Getenv("FORM_IP_RATE_LIMIT"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			log.Fatalf("invalid FORM_IP_RATE_LIMIT %q", v)
		}
		submitLimit = parsed
	}

	staticDir := os.Getenv("FORMS_STATIC_DIR")
	if staticDir == "" {
		// The repo-relative default fits `make forms` (run from the root) and
		// the bare-metal layout (WorkingDirectory=/opt/warmbly).
		staticDir = "forms/dist"
	}

	srv, err := formserver.New(formserver.Config{
		BackendURL:    backendURL,
		InternalToken: token,
		StaticDir:     staticDir,
		SubmitLimit:   submitLimit,
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := srv.Router(splitCSV(os.Getenv("TRUSTED_PROXIES")))
	if err != nil {
		log.Fatalf("invalid TRUSTED_PROXIES: %v", err)
	}

	port := os.Getenv("FORMS_PORT")
	if port == "" {
		port = "8090"
	}
	log.Printf("forms service listening on :%s (backend %s)", port, backendURL)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}

func splitCSV(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
