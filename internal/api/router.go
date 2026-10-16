package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"6eek-go/internal/agents"
	"6eek-go/internal/config"
	"6eek-go/internal/db"
	"6eek-go/internal/events"
	"6eek-go/internal/jobs"
	"6eek-go/internal/media"
	"6eek-go/internal/providers"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Server struct {
	cfg        *config.Config
	database   *db.DB
	broker     *events.Broker
	workers    *jobs.WorkerPool
	agents     *agents.AgentEngine
	openrouter *providers.OpenRouterClient
	media      *media.MediaEngine
	router     *chi.Mux
	ctx        context.Context
	cancel     context.CancelFunc
	tasks      sync.WaitGroup
}

func NewServer(
	cfg *config.Config,
	database *db.DB,
	broker *events.Broker,
	workers *jobs.WorkerPool,
	agentsEngine *agents.AgentEngine,
	openrouter *providers.OpenRouterClient,
	mediaEngine *media.MediaEngine,
) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		ctx: ctx, cancel: cancel,
		cfg:        cfg,
		database:   database,
		broker:     broker,
		workers:    workers,
		agents:     agentsEngine,
		openrouter: openrouter,
		media:      mediaEngine,
		router:     chi.NewRouter(),
	}
	s.agents.SetResolver(s.routes().Agent)
	s.agents.SetLanguageResolver(func() string { return s.routes().Setting("dialogue_language", "English") })
	s.setupRoutes()
	return s
}

func (s *Server) Close() { s.cancel(); s.tasks.Wait() }

func (s *Server) Router() http.Handler {
	return s.router
}

func (s *Server) Database() *db.DB {
	return s.database
}

func (s *Server) setupRoutes() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(5 * time.Minute))

	// CORS matching SvelteKit frontend origins
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:4173", "http://127.0.0.1:4173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Static storage server
	_ = os.MkdirAll(s.cfg.StorageRoot, 0755)
	fileServer := http.FileServer(http.Dir(s.cfg.StorageRoot))
	s.router.Handle("/storage/*", http.StripPrefix("/storage/", fileServer))

	// Health
	s.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status":      "ok",
			"media_ready": s.media.Available(),
			"missing_keys": func() []string {
				if _, err := s.routes().Client("openrouter"); err != nil {
					return []string{"OPENROUTER_API_KEY"}
				}
				return []string{}
			}(),
			"text_model":  s.cfg.OpenRouterTextModel,
			"image_model": s.routes().MediaModel("image"),
			"video_model": s.routes().MediaModel("video"),
			"server":      "6eek-go-v1",
		}
		writeJSON(w, http.StatusOK, resp)
	})

	// Server-Sent Events (SSE)
	s.router.Get("/events", s.handleEvents)

	// Projects
	s.router.Get("/projects", s.handleListProjects)
	s.router.Post("/projects", s.handleCreateProject)
	s.router.Get("/projects/active", s.handleGetActiveProject)
	s.router.Post("/projects/{id}/activate", s.handleActivateProject)
	s.router.Patch("/projects/{id}", s.handleUpdateProject)
	s.router.Delete("/projects/{id}", s.handleDeleteProject)
	s.router.Get("/projects/{id}/export", s.handleExportProject)
	s.router.Post("/projects/{id}/export", s.handleExportProject)
	s.router.Post("/projects/import", s.handleImportProject)

	// Characters
	s.router.Get("/characters", s.handleListCharacters)
	s.router.Post("/characters", s.handleCreateCharacter)
	s.router.Get("/characters/{id}", s.handleGetCharacter)
	s.router.Patch("/characters/{id}", s.handleUpdateCharacter)
	s.router.Delete("/characters/{id}", s.handleDeleteCharacter)
	s.router.Post("/characters/{id}/bible", s.handleCharacterBible)
	s.router.Post("/characters/{id}/reference-sheets", s.handleCharacterRefSheets)

	// Scenes, Shots, Scripts & Ideas
	s.router.Get("/scenes", s.handleListScenes)
	s.router.Post("/scenes", s.handleCreateScene)
	s.router.Get("/scenes/{id}", s.handleGetScene)
	s.router.Patch("/scenes/{id}", s.handleUpdateScene)
	s.router.Delete("/scenes/{id}", s.handleDeleteScene)
	s.router.Post("/scenes/{id}/generate", s.handleExpandScene)
	s.router.Post("/scenes/{id}/refine", s.handleRefineScene)
	s.router.Post("/scenes/{id}/render", s.handleRenderScene)
	s.router.Post("/scenes/{id}/storyboard", s.handleStoryboard)
	s.router.Post("/scenes/{id}/assets/plan", s.handlePlanSceneAssets)
	s.router.Post("/scenes/{id}/assets/generate", s.handleGenerateSceneAssets)
	s.router.Post("/scenes/{id}/shots/generate", s.handleGenerateShots)
	s.router.Post("/scenes/{id}/cast/{character_id}", s.handleCastMemberAdd)
	s.router.Delete("/scenes/{id}/cast/{character_id}", s.handleCastMemberDelete)
	s.router.Get("/scenes/{id}/revisions", s.handleSceneRevisions)
	s.router.Post("/scenes/conversationalize", s.handleConversationalize)

	s.router.Get("/scenes/{id}/shots", s.handleListShots)
	s.router.Post("/scenes/{id}/shots", s.handleCreateShot)
	s.router.Put("/scenes/{id}/shots/order", s.handleReorderShots)
	s.router.Patch("/shots/{id}", s.handleUpdateShot)
	s.router.Delete("/shots/{id}", s.handleDeleteShot)
	s.router.Post("/shots/{id}/refine", s.handleRefineShot)
	s.router.Post("/shots/{id}/assets/{asset_id}", s.handleShotAssetAttach)
	s.router.Delete("/shots/{id}/assets/{asset_id}", s.handleShotAssetDetach)
	s.router.Get("/shots/{id}/revisions", s.handleShotRevisions)
	s.router.Post("/revisions/{id}/revert", s.handleRevertRevision)

	s.router.Post("/ideas/develop", s.handleDevelopIdea)
	s.router.Post("/scripts/generate", s.handleGenerateScript)
	s.router.Get("/scripts", s.handleListScripts)

	// Assets
	s.router.Get("/assets", s.handleListAssets)
	s.router.Get("/assets/types", s.handleAssetTypes)
	s.router.Post("/assets/upload", s.handleUploadAsset)
	s.router.Get("/assets/{id}", s.handleGetAsset)
	s.router.Delete("/assets/{id}", s.handleDeleteAsset)
	s.router.Post("/assets/{id}/recognise", s.handleRecogniseAsset)

	// Render & Outputs
	s.router.Post("/render", s.handleStartRender)
	s.router.Post("/render/from-shot", s.handleRenderFromShot)
	s.router.Get("/render-jobs", s.handleListRenderJobs)
	s.router.Get("/render-jobs/{id}", s.handleGetRenderJob)
	s.router.Post("/render-jobs/{id}/resubmit", s.handleResubmitJob)

	s.router.Get("/outputs/{id}/editor", s.handleGetEditorData)
	s.router.Post("/outputs/{id}/retry", s.handleRetryOutput)
	s.router.Patch("/outputs/{id}/select", s.handleSelectOutput)
	s.router.Get("/outputs/{id}/download", s.handleDownloadOutput)
	s.router.Post("/outputs/{id}/run-qa", s.handleRunQA)
	s.router.Get("/outputs/{id}/captions", s.handleGetCaptions)
	s.router.Put("/outputs/{id}/captions", s.handleUpdateCaptions)
	s.router.Post("/outputs/{id}/captions/transcribe", s.handleTranscribeCaptions)
	s.router.Post("/outputs/{id}/caption", s.handleCaptionOutput)
	s.router.Get("/caption-styles", s.handleCaptionStyles)
	s.router.Get("/caption-config", s.handleCaptionConfig)

	// Style Guide
	s.router.Get("/style", s.handleGetStyle)
	s.router.Patch("/style", s.handleUpdateStyle)
	s.router.Put("/style", s.handleUpdateStyle)
	s.router.Post("/style/ingest", s.handleIngestStyle)

	// Settings
	s.router.Get("/settings/app", s.handleGetAppSettings)
	s.router.Put("/settings/app", s.handlePutAppSettings)
	s.router.Get("/settings/agents", s.handleListAgents)
	s.router.Put("/settings/agents/{agent}", s.handlePutAgent)
	s.router.Get("/settings/providers", s.handleListProviders)
	s.router.Get("/settings/providers/{name}", s.handleGetProviderConfig)
	s.router.Put("/settings/providers/{name}", s.handlePutProviderConfig)
	s.router.Post("/settings/providers/{name}/test", s.handleTestProvider)
	s.router.Get("/settings/providers/{name}/models", s.handleDiscoverModels)
	s.router.Post("/settings/providers/{name}/verify-model", s.handleVerifyModel)
	s.router.Get("/settings/connection-presets", s.handleConnectionPresets)
	s.router.Post("/settings/connections", s.handleCreateConnection)

	// Chat Assistant
	s.router.Post("/chat", s.handleChat)

	// Videos (Guided Pipeline)
	s.router.Get("/videos/preflight", s.handleVideoPreflight)
	s.router.Post("/videos/generate", s.handleGenerateVideo)

	// Ops
	s.router.Get("/ops/{id}", s.handleGetOp)
	s.router.Get("/ops", s.handleListOps)

	// Graph Canvas
	s.router.Get("/graph", s.handleGetGraph)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
