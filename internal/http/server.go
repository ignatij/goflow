// internal/http/server.go
package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ignatij/goflow/internal/log"
	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
)

func StartServer(port string, store storage.Store) error {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/workflows", workflowsHandler(store))

	log.GetLogger().Infof("Starting GoFlow server on :%s", port)
	return http.ListenAndServe(":"+port, nil)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "GoFlow server is running")
}

func workflowsHandler(store storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listWorkflowsHTTP(w, r, store)
		case http.MethodPost:
			createWorkflowHTTP(w, r, store)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func createWorkflowHTTP(w http.ResponseWriter, r *http.Request, store storage.Store) {
	wf := models.Workflow{
		Name:      r.FormValue("name"),
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if wf.Name == "" {
		log.GetLogger().Error("Missing 'name' parameter in POST /workflows")
		http.Error(w, "Missing 'name' parameter", http.StatusBadRequest)
		return
	}
	id, err := store.SaveWorkflow(wf)
	if err != nil {
		log.GetLogger().Errorf("Failed to create workflow: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create workflow: %v", err), http.StatusInternalServerError)
		return
	}
	log.GetLogger().Infof("Created workflow '%s' with ID %d", wf.Name, id)
	fmt.Fprintf(w, "Created workflow '%s' with ID %d\n", wf.Name, id)
}

func listWorkflowsHTTP(w http.ResponseWriter, r *http.Request, store storage.Store) {
	// lint issue
	_ = r
	workflows, err := store.ListWorkflows()
	if err != nil {
		log.GetLogger().Errorf("Failed to list workflows: %v", err)
		http.Error(w, fmt.Sprintf("Failed to list workflows: %v", err), http.StatusInternalServerError)
		return
	}
	if len(workflows) == 0 {
		fmt.Fprintf(w, "No workflows found.\n")
		return
	}
	for _, wf := range workflows {
		fmt.Fprintf(w, "- ID: %d, Name: %s, Status: %s, Created: %s\n", wf.ID, wf.Name, wf.Status, wf.CreatedAt.Format(time.RFC3339))
	}
}
