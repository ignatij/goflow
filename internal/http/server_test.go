package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ignatij/goflow/internal/service"
	internal "github.com/ignatij/goflow/internal/storage"
	"github.com/ignatij/goflow/internal/testutil"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestE2EServer(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Teardown(t)

	newServer := func(store storage.Store) *httptest.Server {
		svc := service.NewWorkflowService(store)
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				healthHandler(w, r)
			case "/workflows":
				workflowsHandler(svc)(w, r)
			default:
				http.NotFound(w, r)
			}
		}))
	}

	newTestStore := func(t *testing.T) storage.Store {
		store, err := internal.InitStore(testDB.ConnStr)
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

		data := url.Values{"name": {"test-workflow"}}
		resp, err := srv.Client().PostForm(srv.URL+"/workflows", data)
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "{\"id\":1,\"message\":\"Created workflow 'test-workflow' with ID 1\"}\n", string(body))
	})

	t.Run("ListWorkflows", func(t *testing.T) {
		store := newTestStore(t)
		srv := newServer(store)
		defer srv.Close()

		data := url.Values{"name": {"test-workflow"}}
		_, err := srv.Client().PostForm(srv.URL+"/workflows", data)
		assert.NoError(t, err)

		resp, err := srv.Client().Get(srv.URL + "/workflows")
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "{\"id\":1,\"name\":\"test-workflow\",\"status\":\"pending\"")
	})

	t.Run("CreateWorkflowMissingName", func(t *testing.T) {
		store := newTestStore(t)
		srv := newServer(store)
		defer srv.Close()

		resp, err := srv.Client().Post(srv.URL+"/workflows", "application/x-www-form-urlencoded", nil)
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
}
