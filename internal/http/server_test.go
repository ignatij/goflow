package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	internal_http "github.com/ignatij/goflow/internal/http"
	"github.com/ignatij/goflow/internal/log"
	internal_storage "github.com/ignatij/goflow/internal/storage"
	"github.com/ignatij/goflow/internal/testutil"
	"github.com/ignatij/goflow/pkg/service"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestE2EServer(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Teardown(t)

	newServer := func(store storage.Store) *httptest.Server {
		svc := service.NewWorkflowService(store, log.GetLogger())
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				internal_http.HealthHandler(w, r)
			case "/workflows":
				internal_http.WorkflowsHandler(svc)(w, r)
			default:
				http.NotFound(w, r)
			}
		}))
	}

	newTestStore := func(t *testing.T) storage.Store {
		store, err := internal_storage.InitStore(testDB.ConnStr)
		assert.NoError(t, err)
		t.Cleanup(func() {
			_, err := testDB.DB.Exec("TRUNCATE TABLE workflows RESTART IDENTITY CASCADE")
			assert.NoError(t, err)
			store.Close()
		})
		return store
	}

	t.Run("HealthCheck", func(t *testing.T) {
		store := newTestStore(t)
		srv := newServer(store)
		defer srv.Close()

		resp, err := srv.Client().Get(srv.URL + "/health")
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "GoFlow server is running", string(body))
	})

	t.Run("CreateWorkflow", func(t *testing.T) {
		store := newTestStore(t)
		srv := newServer(store)
		defer srv.Close()

		// Prepare JSON payload
		jsonData := []byte(`{"name": "test-workflow"}`)
		req, err := http.NewRequest("POST", srv.URL+"/workflows", bytes.NewBuffer(jsonData))
		assert.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Send request
		resp, err := srv.Client().Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		// Verify response
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, `{"id":1,"message":"Created workflow 'test-workflow' with ID 1"}`+"\n", string(body))
	})

	t.Run("ListWorkflows", func(t *testing.T) {
		store := newTestStore(t)
		srv := newServer(store)
		defer srv.Close()

		// Prepare JSON payload
		jsonData := []byte(`{"name": "test-workflow"}`)
		req, err := http.NewRequest("POST", srv.URL+"/workflows", bytes.NewBuffer(jsonData))
		assert.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Send request
		resp, err := srv.Client().Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		resp, err = srv.Client().Get(srv.URL + "/workflows")
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "{\"id\":1,\"name\":\"test-workflow\",\"status\":\"PENDING\"")
	})

	t.Run("CreateWorkflowMissingName", func(t *testing.T) {
		store := newTestStore(t)
		srv := newServer(store)
		defer srv.Close()

		// Prepare JSON payload
		jsonData := []byte(`{"name": ""}`)
		req, err := http.NewRequest("POST", srv.URL+"/workflows", bytes.NewBuffer(jsonData))
		assert.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Send request
		resp, err := srv.Client().Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "{\"error\":\"Missing 'name' parameter\"}\n", string(body))
	})

	t.Run("ListEmptyWorkflows", func(t *testing.T) {
		store := newTestStore(t)
		srv := newServer(store)
		defer srv.Close()

		resp, err := srv.Client().Get(srv.URL + "/workflows")
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "[]\n", string(body))
	})

	t.Run("UpdateWorkflowStatus", func(t *testing.T) {
		store := newTestStore(t)
		srv := newServer(store)
		defer srv.Close()

		// Prepare JSON payload
		jsonData := []byte(`{"name": "test-workflow"}`)
		req, err := http.NewRequest("POST", srv.URL+"/workflows", bytes.NewBuffer(jsonData))
		assert.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Send request
		resp, err := srv.Client().Do(req)
		assert.NoError(t, err)
		body, err := io.ReadAll(resp.Body)

		var response struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatalf("Failed to unmarshall response: %v", err)
		}

		assert.NoError(t, err)
		defer resp.Body.Close()

		id := strconv.FormatInt(response.ID, 10)
		jsonData = []byte(fmt.Sprintf(`{"id": %d, "status": "COMPLETED"}`, response.ID))
		req, err = http.NewRequest("PUT", srv.URL+"/workflows", bytes.NewBuffer(jsonData))
		assert.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err = srv.Client().Do(req)
		assert.NoError(t, err)

		assert.Equal(t, 200, resp.StatusCode)

		body, err = io.ReadAll(resp.Body)
		assert.NoError(t, err)
		defer resp.Body.Close()

		var responseUpdateWorkflowStatus struct {
			ID      int64  `json:"id"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &responseUpdateWorkflowStatus); err != nil {
			t.Fatalf("Failed to unmarshall response: %v", err)
		}

		assert.Equal(t, response.ID, responseUpdateWorkflowStatus.ID)
		assert.Equal(t, "Updated the status to 'COMPLETED' of the workflow with ID: "+id, responseUpdateWorkflowStatus.Message)
	})
}
