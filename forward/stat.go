package forward

import (
	"context"
	"net/http"
)

var (
	ctxKeyFwdStat = &Stat{}
)

type Stat struct {
	TargetRequest *http.Request
}

func RequestWithStat(r *http.Request) *Stat {
	s, _ := r.Context().Value(ctxKeyFwdStat).(*Stat)
	return s
}

func RequestAddStat(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKeyFwdStat, &Stat{}))
}
