package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name            string
		err             error
		wantHTTPStatus  int
		wantCode        int
		wantGinErrCount int
	}{
		{
			name:           "业务错误保留原错误码",
			err:            apperr.New(consts.CodeParamError),
			wantHTTPStatus: http.StatusOK,
			wantCode:       consts.CodeParamError,
		},
		{
			name:            "未知错误归一化为服务端错误",
			err:             errors.New("upstream failed"),
			wantHTTPStatus:  http.StatusInternalServerError,
			wantCode:        consts.CodeInternalError,
			wantGinErrCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			handleServiceError(c, tt.err)

			var body struct {
				Code int `json:"code"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tt.wantHTTPStatus, w.Code)
			assert.Equal(t, tt.wantCode, body.Code)
			assert.Len(t, c.Errors, tt.wantGinErrCount)
		})
	}
}
