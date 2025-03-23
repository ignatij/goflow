// internal/http/server_test.go
package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	internal_storage "github.com/ignatij/goflow/internal/storage"
	"github.com/ignatij/goflow/internal/testutil"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
)

type txWrapper struct {
	*sqlx.Tx
}

func (w *txWrapper) Close() error {
	return w.Rollback()
}

func TestE2EServer(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Teardown(t)

	newServer := func(store storage.Store) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				healthHandler(w, r)
			case "/workflows":
				workflowsHandler(store)(w, r)
			default:
				http.NotFound(w, r)
			}
		}))
	}

	// Test 1: Health check
	t.Run("HealthCheck", func(t *testing.T) {
		tx, err := testDB.DB.Beginx()
		if err != nil {
			t.Fatalf("Error initiating transaction: %v", err)
		}
		t.Cleanup(func() {
			tx.Rollback()
		})
		srv := newServer(internal_storage.NewPostgresStoreWithTx(&txWrapper{tx}))
		defer srv.Close()
		client := srv.Client()
		resp, err := client.Get(srv.URL + "/health")
		if err != nil {
			t.Fatalf("Failed to send GET request: %v", err)
		}
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK")
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "GoFlow server is running", string(body), "Expected health message")
	})

	// Test 2: Create a workflow
	t.Run("CreateWorkflow", func(t *testing.T) {
		tx, err := testDB.DB.Beginx()
		if err != nil {
			t.Fatalf("Error initiating transaction: %v", err)
		}
		t.Cleanup(func() {
			tx.Rollback()
		})
		srv := newServer(internal_storage.NewPostgresStoreWithTx(&txWrapper{tx}))
		defer srv.Close()
		client := srv.Client()

		data := url.Values{}
		data.Set("name", "test-workflow")
		resp, err := client.PostForm(srv.URL+"/workflows", data)
		if err != nil {
			t.Fatalf("Failed to send POST request: %v", err)
		}
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK")
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "Created workflow 'test-workflow' with ID", "Expected creation message")
	})

	// Test 3: List workflows
	t.Run("ListWorkflows", func(t *testing.T) {
		tx, err := testDB.DB.Beginx()
		if err != nil {
			t.Fatalf("Error initiating transaction: %v", err)
		}
		t.Cleanup(func() {
			tx.Rollback()
		})
		srv := newServer(internal_storage.NewPostgresStoreWithTx(&txWrapper{tx}))
		defer srv.Close()
		client := srv.Client()

		// create workflow
		data := url.Values{}
		data.Set("name", "test-workflow")
		resp, err := client.PostForm(srv.URL+"/workflows", data)
		if err != nil {
			t.Fatalf("Failed to send POST request: %v", err)
		}
		defer resp.Body.Close()

		resp2, err := client.Get(srv.URL + "/workflows")
		if err != nil {
			t.Fatalf("Failed to send GET request: %v", err)
		}
		defer resp2.Body.Close()

		assert.Equal(t, http.StatusOK, resp2.StatusCode, "Expected 200 OK")
		body, _ := io.ReadAll(resp2.Body)
		assert.Contains(t, string(body), "Name: test-workflow, Status: pending", "Expected workflow in list")
	})

	// Test 4: Create workflow with missing name
	t.Run("CreateWorkflowMissingName", func(t *testing.T) {
		tx, err := testDB.DB.Beginx()
		if err != nil {
			t.Fatalf("Error initiating transaction: %v", err)
		}
		t.Cleanup(func() {
			tx.Rollback()
		})
		srv := newServer(internal_storage.NewPostgresStoreWithTx(&txWrapper{tx}))
		defer srv.Close()
		client := srv.Client()
		resp, err := client.Post(srv.URL+"/workflows", "application/x-www-form-urlencoded", nil)
		if err != nil {
			t.Fatalf("Failed to send POST request: %v", err)
		}
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Expected 400 Bad Request")
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "Missing 'name' parameter\n", string(body), "Expected error message")
	})

	// Test 5: List empty workflows
	t.Run("ListEmptyWorkflows", func(t *testing.T) {
		tx, err := testDB.DB.Beginx()
		if err != nil {
			t.Fatalf("Error initiating transaction: %v", err)
		}
		t.Cleanup(func() {
			tx.Rollback()
		})
		srv := newServer(internal_storage.NewPostgresStoreWithTx(&txWrapper{tx}))
		defer srv.Close()
		client := srv.Client()

		resp, err := client.Get(srv.URL + "/workflows")
		if err != nil {
			t.Fatalf("Failed to send GET request: %v", err)
		}
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK")
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "No workflows found.\n", string(body), "Expected empty message")
	})
}
