package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"docforge/internal/config"
	"docforge/internal/repository"
	"docforge/internal/web"
)

func NewRouter(cfg *config.Config, repo *repository.Repository, handlers *Handlers, authHandler *AuthHandler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Configure CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:3000", "http://127.0.0.1:5173", "*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link", "Content-Disposition"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	webHandler := web.Handler()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": "1.0.0",
		})
	})

	r.Get("/", webHandler.ServeHTTP)

	r.Route("/api/v1", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "api endpoint not found"})
		})

		// Public Auth routes
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", authHandler.Login)
			r.Get("/atlassian/start", authHandler.AtlassianSSOStart)
			r.Get("/atlassian/callback", authHandler.AtlassianSSOCallback)
			r.Get("/sso/microsoft", authHandler.MicrosoftSSOStart)
			r.Get("/sso/callback", authHandler.MicrosoftSSOCallback)

			r.Group(func(r chi.Router) {
				r.Use(AuthMiddleware(cfg, repo))
				r.Get("/me", authHandler.Me)
			})
		})

		// Protected API routes
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(cfg, repo))

			// LLM Configs (Available for project association and selection)
			r.Get("/llm/configs", handlers.ListAvailableLLMConfigs)

			// Admin Group
			r.Route("/admin", func(r chi.Router) {
				r.Use(RequireAdmin)
				r.Get("/users", handlers.AdminListUsers)
				r.Post("/users", handlers.AdminCreateUser)
				r.Put("/users/{id}", handlers.AdminUpdateUser)

				r.Get("/llm", handlers.AdminListLLMConfigs)
				r.Post("/llm", handlers.AdminSaveLLMConfig)
			})

			// Projects
			r.Route("/projects", func(r chi.Router) {
				r.Get("/", handlers.ListProjects)
				r.Post("/", handlers.CreateProject)
				r.Get("/{id}", handlers.GetProject)
				r.Put("/{id}", handlers.UpdateProject)
				r.Delete("/{id}", handlers.DeleteProject)

				// Project Members
				r.Get("/{id}/members", handlers.ListProjectMembers)
				r.Post("/{id}/members", handlers.AddProjectMember)
				r.Delete("/{id}/members/{memberId}", handlers.DeleteProjectMember)

				// Project Sources
				r.Get("/{id}/sources", handlers.ListSources)
				r.Post("/{id}/sources/upload", handlers.UploadSourceFile)
				r.Post("/{id}/sources/remote", handlers.AddRemoteSource)

				// Project Generations
				r.Get("/{id}/generations", handlers.ListGenerations)
				r.Post("/{id}/generations", handlers.SubmitGeneration)

				// Project Documents
				r.Get("/{id}/documents", handlers.ListDocuments)

				// Completeness Pre-Flight Scorecard
				r.Get("/{id}/completeness", handlers.GetProjectCompleteness)

				// Token Usage for Chargeback
				r.Get("/{id}/token-usage", handlers.GetProjectTokenUsage)

				// Confluence Target & Hierarchy for Project
				r.Get("/{id}/confluence-target", handlers.GetProjectConfluenceTarget)
				r.Post("/{id}/confluence-target", handlers.SetProjectConfluenceTarget)
				r.Delete("/{id}/confluence-target", handlers.DeleteProjectConfluenceTarget)
				r.Get("/{id}/confluence/sites", handlers.ListProjectConfluenceSites)
				r.Get("/{id}/confluence/spaces", handlers.ListProjectConfluenceSpaces)
				r.Get("/{id}/confluence/pages", handlers.ListProjectConfluencePages)
			})

			// Sources direct delete
			r.Delete("/sources/{id}", handlers.DeleteSource)

			// Generations direct query & stages
			r.Get("/generations/{id}", handlers.GetGeneration)
			r.Get("/generations/{id}/stages", handlers.GetGenerationStages)

			// Documents
			r.Route("/documents", func(r chi.Router) {
				r.Get("/{id}", handlers.GetDocument)
				r.Put("/{id}", handlers.UpdateDocument)
				r.Post("/{id}/approve", handlers.ApproveDocument)
				r.Get("/{id}/download", handlers.DownloadDocumentMarkdown)
				r.Post("/{id}/export-confluence", handlers.ExportDocumentToConfluence)
				r.Post("/{id}/feedback", handlers.AddFeedback)
				r.Post("/{id}/update-with-feedback", handlers.UpdateWithFeedback)
			})

			// Feedback status updates
			r.Put("/feedback/{fbId}", handlers.UpdateFeedbackStatus)

			// Atlassian Integration
			r.Route("/atlassian", func(r chi.Router) {
				r.Get("/connect", handlers.AtlassianConnect)
				r.Get("/callback", handlers.AtlassianCallback)
				r.Get("/sites", handlers.AtlassianListSites)
				r.Get("/spaces", handlers.AtlassianListSpaces)
			})

			// Notification Preferences
			r.Route("/notifications", func(r chi.Router) {
				r.Get("/preferences", handlers.GetNotificationPreferences)
				r.Post("/preferences", handlers.SaveNotificationPreference)
			})
		})
	})

	r.NotFound(webHandler.ServeHTTP)

	return r
}
