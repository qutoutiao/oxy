package forward

import (
	"context"
	"net/http"
)

var (
	ctxKeyFwdExport = &Export{}
)

// Export Export info
type Export struct {
	targetRequest  *http.Request              // 请求到后端前的 request
	modifyResponse func(*http.Response) error // 返回到前端前修改 response
}

// RequestGetExport 经过 proxy 后可从 request 中取出 Export
func RequestGetExport(r *http.Request) *Export {
	s, _ := r.Context().Value(ctxKeyFwdExport).(*Export)
	return s
}

// RequestAddExport 添加请求到后端前的 request 的注入
func RequestAddExport(r *http.Request) (*http.Request, *Export) {
	e := RequestGetExport(r)
	if e != nil {
		return r, e
	}
	e = &Export{}
	return r.WithContext(context.WithValue(r.Context(), ctxKeyFwdExport, e)), e
}

// GetTargetRequest 获得请求到后端前的 request
func (e *Export) GetTargetRequest() *http.Request {
	return e.targetRequest
}

// SetModifyResponse 设置请求返回到前端前的修改函数
func (e *Export) SetModifyResponse(modifyResponse func(*http.Response) error) {
	e.modifyResponse = modifyResponse
}
