package forward

import (
	"context"
	"net/http"
)

var (
	ctxKeyFwdStat = &Stat{}
)

// Stat stat hook
type Stat struct {
	TargetRequest *http.Request
}

// RequestGetStat get stat
func RequestGetStat(r *http.Request) *Stat {
	s, _ := r.Context().Value(ctxKeyFwdStat).(*Stat)
	if s != nil && s.TargetRequest == nil {
		return nil
	}
	return s
}

// RequestAddStat add stat
func RequestAddStat(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKeyFwdStat, &Stat{}))
}
