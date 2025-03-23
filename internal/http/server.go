package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ignatij/goflow/internal/log"
	"github.com/ignatij/goflow/internal/service"
	"github.com/ignatij/goflow/pkg/storage"
)

func StartServer(port string, store storage.Store) error {
	svc := service.NewWorkflowService(store)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/workflows", workflowsHandler(svc))

	log.GetLogger().Infof("Starting GoFlow server on :%s", port)
	return http.ListenAndServe(":"+port, nil)
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
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func createWorkflowHTTP(w http.ResponseWriter, r *http.Request, svc *service.WorkflowService) {
	name := r.FormValue("name")
	if name == "" {
		log.GetLogger().Error("Missing 'name' parameter in POST /workflows")
		http.Error(w, "Missing 'name' parameter", http.StatusBadRequest)
		return
	}
	id, err := svc.CreateWorkflow(name)
	if err != nil {
		log.GetLogger().Errorf("Failed to create workflow: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create workflow: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Created workflow '%s' with ID %d\n", name, id)
}

func listWorkflowsHTTP(w http.ResponseWriter, r *http.Request, svc *service.WorkflowService) {
	_ = r
	workflows, err := svc.ListWorkflows()
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
