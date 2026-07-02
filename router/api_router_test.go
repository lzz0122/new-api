package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelRootRoutesAcceptNoTrailingSlash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("new-api-test", cookie.NewStore([]byte("test-secret"))))
	SetApiRouter(engine)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/channel"},
		{method: http.MethodGet, path: "/api/channel/"},
		{method: http.MethodPost, path: "/api/channel", body: "{}"},
		{method: http.MethodPost, path: "/api/channel/", body: "{}"},
		{method: http.MethodPut, path: "/api/channel", body: "{}"},
		{method: http.MethodPut, path: "/api/channel/", body: "{}"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, req)

			require.NotEqual(t, http.StatusNotFound, recorder.Code)
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}
