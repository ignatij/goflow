package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ignatij/goflow/internal/log"
	"github.com/ignatij/goflow/internal/service"
	"github.com/ignatij/goflow/pkg/storage"
)

// StartServer runs the GoFlow HTTP server with graceful shutdown.
// It returns an error if the server fails to start or shut down cleanly.
func StartServer(port string, store storage.Store) error {
	svc := service.NewWorkflowService(store)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/workflows", workflowsHandler(svc))

	srv := &http.Server{Addr: ":" + port}
	errChan := make(chan error, 1)

	log.GetLogger().Infof("Starting GoFlow server on :%s", port)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
		close(errChan)
	}()

	// Wait for shutdown signal or server error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		if err != nil {
			log.GetLogger().Errorf("Server failed: %v", err)
			return err
		}
	case <-sigChan:
		log.GetLogger().Info("Received shutdown signal")
	}

	// Perform graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.GetLogger().Errorf("Shutdown failed: %v", err)
		return err
	}

	// Check for any late-arriving server errors
	if err := <-errChan; err != nil {
		log.GetLogger().Errorf("Server failed during operation: %v", err)
		return err
	}

	log.GetLogger().Info("Server shut down cleanly")
	return nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "GoFlow server is running")
}

func workflowsHandler(svc *service.WorkflowService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listWorkflowsHTTP(w, r, svc)
		case http.MethodPost:
			createWorkflowHTTP(w, r, svc)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			err := json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
			if err != nil {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}
	}
}

func createWorkflowHTTP(w http.ResponseWriter, r *http.Request, svc *service.WorkflowService) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.GetLogger().Errorf("Failed to read body: %v", err)
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}
	// Parse JSON
	var input struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		log.GetLogger().Errorf("Failed to parse JSON: %v", err)
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if input.Name == "" {
		http.Error(w, `{"error":"Missing 'name' parameter"}`, http.StatusBadRequest)
		return
	}
	id, err := svc.CreateWorkflow(input.Name)
	if err != nil {
		log.GetLogger().Errorf("Failed to create workflow: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to create workflow: %v", err)}); err != nil {
			log.GetLogger().Errorf("Failed to encode error response: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"message": fmt.Sprintf("Created workflow '%s' with ID %d", input.Name, id),
		"id":      id,
	}); err != nil {
		log.GetLogger().Errorf("Failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func listWorkflowsHTTP(w http.ResponseWriter, r *http.Request, svc *service.WorkflowService) {
	// lint issue
	_ = r
	workflows, err := svc.ListWorkflows()
	if err != nil {
		log.GetLogger().Errorf("Failed to list workflows: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		err := json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to list workflows: %v", err)})
		if err != nil {
			http.Error(w, "Failed to list workflows", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(workflows); err != nil {
		log.GetLogger().Errorf("Failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
