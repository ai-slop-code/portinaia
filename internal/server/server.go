package server

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

	webassets "portinaia/web"
)

type healthResponse struct {
	Status string `json:"status"`
}

func NewHandler() http.Handler {
	router := chi.NewRouter()
	router.Route("/api/v1", func(api chi.Router) {
		api.Get("/health/live", healthHandler)
		api.Get("/health/ready", healthHandler)
		api.NotFound(apiNotFoundHandler)
	})
	router.Get("/*", frontendHandler())
	return router
}

func healthHandler(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(healthResponse{Status: "ok"})
}

func apiNotFoundHandler(response http.ResponseWriter, _ *http.Request) {
	http.Error(response, "not found", http.StatusNotFound)
}

func frontendHandler() http.HandlerFunc {
	assets := webassets.Assets()
	files := http.FileServer(http.FS(assets))

	return func(response http.ResponseWriter, request *http.Request) {
		path := request.URL.Path[1:]
		if path != "" {
			if info, err := fs.Stat(assets, path); err == nil && !info.IsDir() {
				files.ServeHTTP(response, request)
				return
			}
		}

		index, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(response, "frontend unavailable", http.StatusInternalServerError)
			return
		}

		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = response.Write(index)
	}
}
