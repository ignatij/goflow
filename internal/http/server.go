package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ignatij/goflow/internal/log"
	"github.com/ignatij/goflow/pkg/service"
	"github.com/ignatij/goflow/pkg/storage"
)

// StartServer runs the GoFlow HTTP server with graceful shutdown.
// It returns an error if the server fails to start or shut down cleanly.
func StartServer(port string, store storage.Store) error {
	ctx := context.Background()
	svc := service.NewWorkflowService(ctx, store, log.GetLogger())
	http.HandleFunc("/health", HealthHandler)
	http.HandleFunc("/workflows", WorkflowsHandler(svc))
	http.HandleFunc("/workflows/", WorkflowByIDHandler(svc))

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
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
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

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "GoFlow server is running")
}

func WorkflowsHandler(svc *service.WorkflowService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listWorkflowsHTTP(w, r, svc)
		case http.MethodPost:
			createWorkflowHTTP(w, r, svc)
		case http.MethodPut:
			updateWorkflowStatusHTTP(w, r, svc)
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

func WorkflowByIDHandler(svc *service.WorkflowService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract workflow ID from path
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 3 {
			http.Error(w, `{"error":"Invalid URL format"}`, http.StatusBadRequest)
			return
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			http.Error(w, `{"error":"Invalid workflow ID"}`, http.StatusBadRequest)
			return
		}

		// Check for flow execution (e.g., /workflows/123/flows/report)
		if r.URL.Path != fmt.Sprintf("/workflows/%d", id) {
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) == 5 && parts[3] == "flows" && r.Method == http.MethodPost {
				flowName := parts[4]
				executeFlowHTTP(w, r, svc, id, flowName)
				return
			}
			http.Error(w, `{"error":"Invalid URL"}`, http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
			return
		}

		getWorkflowHTTP(w, r, svc, id)
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

func updateWorkflowStatusHTTP(w http.ResponseWriter, r *http.Request, svc *service.WorkflowService) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.GetLogger().Errorf("Failed to read body: %v", err)
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}
	// Parse JSON
	var input struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		log.GetLogger().Errorf("Failed to parse JSON: %v", err)
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if input.ID == 0 {
		http.Error(w, `{"error":"Missing 'id' parameter"}`, http.StatusBadRequest)
		return
	}
	if input.Status == "" {
		http.Error(w, `{"error":"Missing 'status' parameter"}`, http.StatusBadRequest)
		return
	}
	err = svc.UpdateWorkflowStatus(input.ID, input.Status)
	if err != nil {
		log.GetLogger().Errorf("Failed to update workflow status: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to update workflow status: %v", err)}); err != nil {
			log.GetLogger().Errorf("Failed to encode error response: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"message": fmt.Sprintf("Updated the status to '%s' of the workflow with ID: %d", input.Status, input.ID),
		"id":      input.ID,
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

func getWorkflowHTTP(w http.ResponseWriter, r *http.Request, svc *service.WorkflowService, id int64) {
	// lint issue
	_ = r
	wf, err := svc.GetWorkflow(id)
	if err != nil {
		log.GetLogger().Errorf("Failed to get workflow %d: %v", id, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to get workflow: %v", err)})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(wf)
}

func executeFlowHTTP(w http.ResponseWriter, r *http.Request, svc *service.WorkflowService, workflowID int64, flowName string) {
	// lint issue
	_ = r
	result, err := svc.ExecuteFlow(r.Context(), workflowID, flowName)
	if err != nil {
		log.GetLogger().Errorf("Failed to execute flow '%s' for workflow %d: %v", flowName, workflowID, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to execute flow: %v", err)})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"result": result,
	})
}
