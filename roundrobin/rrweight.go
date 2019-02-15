// Package roundrobin implements dynamic weighted round robin load balancer http handler
package roundrobin

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qutoutiao/oxy/utils"
	log "github.com/sirupsen/logrus"
)

// LBOption provides options for load balancer
type WeightLBOption func(*RoundRobinWeight) error

// RoundRobinWeightErrorHandler is a functional argument that sets error handler of the server
func RoundRobinWeightErrorHandler(h utils.ErrorHandler) WeightLBOption {
	return func(s *RoundRobinWeight) error {
		s.errHandler = h
		return nil
	}
}

// RoundRobinWeightEnableStickySession enable sticky session
func RoundRobinWeightEnableStickySession(stickySession *StickySession) WeightLBOption {
	return func(s *RoundRobinWeight) error {
		s.stickySession = stickySession
		return nil
	}
}

// RoundRobinWeight implements dynamic weighted round robin load balancer http handler
type RoundRobinWeight struct {
	mutex      sync.RWMutex
	next       http.Handler
	errHandler utils.ErrorHandler

	servers       []*server
	stickySession *StickySession
	serverList    atomic.Value // *serverList

	log *log.Logger
}

// NewRoundRobinWeight created a new RoundRobinWeight
func NewRoundRobinWeight(next http.Handler, opts ...WeightLBOption) (*RoundRobinWeight, error) {
	rr := &RoundRobinWeight{
		next:    next,
		servers: []*server{},

		log: log.StandardLogger(),
	}
	for _, o := range opts {
		if err := o(rr); err != nil {
			return nil, err
		}
	}
	if rr.errHandler == nil {
		rr.errHandler = utils.DefaultHandler
	}
	return rr, nil
}

// RoundRobinWeightLogger defines the logger the round robin load balancer will use.
//
// It defaults to logrus.StandardLogger(), the global logger used by logrus.
func RoundRobinWeightLogger(l *log.Logger) WeightLBOption {
	return func(r *RoundRobinWeight) error {
		r.log = l
		return nil
	}
}

// Next returns the next handler
func (r *RoundRobinWeight) Next() http.Handler {
	return r.next
}

func (r *RoundRobinWeight) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if r.log.Level >= log.DebugLevel {
		logEntry := r.log.WithField("Request", utils.DumpHttpRequest(req))
		logEntry.Debug("vulcand/oxy/roundrobin/rr: begin ServeHttp on request")
		defer logEntry.Debug("vulcand/oxy/roundrobin/rr: completed ServeHttp on request")
	}

	// make shallow copy of request before chaning anything to avoid side effects
	newReq := *req
	stuck := false
	if r.stickySession != nil {
		cookieURL, present, err := r.stickySession.GetBackend(&newReq, r.Servers())

		if err != nil {
			log.Warnf("vulcand/oxy/roundrobin/rr: error using server from cookie: %v", err)
		}

		if present {
			newReq.URL = cookieURL
			stuck = true
		}
	}

	if !stuck {
		url, err := r.NextServer()
		if err != nil {
			r.errHandler.ServeHTTP(w, req, err)
			return
		}

		if r.stickySession != nil {
			r.stickySession.StickBackend(url, &w)
		}
		newReq.URL = url
	}

	if r.log.Level >= log.DebugLevel {
		// log which backend URL we're sending this request to
		r.log.WithFields(log.Fields{"Request": utils.DumpHttpRequest(req), "ForwardURL": newReq.URL}).Debugf("vulcand/oxy/roundrobin/rr: Forwarding this request to URL")
	}

	r.next.ServeHTTP(w, &newReq)
}

// NextServer gets the next server
func (r *RoundRobinWeight) NextServer() (*url.URL, error) {
	s, _ := r.serverList.Load().(*serverList)
	if s == nil {
		return nil, fmt.Errorf("no servers in the pool")
	}
	u := s.nextServer()
	if u == nil {
		return nil, fmt.Errorf("no servers in the pool")
	}
	return utils.CopyURL(u), nil
}

// UpsertServer In case if server is already present in the load balancer, returns error
func (r *RoundRobinWeight) UpsertServer(u *url.URL, options ...ServerOption) error {
	if u == nil {
		return fmt.Errorf("server URL can't be nil")
	}
	srv := &server{url: utils.CopyURL(u)}
	for _, o := range options {
		if err := o(srv); err != nil {
			return err
		}
	}

	if srv.weight == 0 {
		srv.weight = defaultWeight
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if s, _ := r.findServerByURL(u); s != nil {
		for _, o := range options {
			if err := o(s); err != nil {
				return err
			}
		}
		if s.weight == 0 {
			s.weight = defaultWeight
		}
		r.resetState()
		return nil
	}

	r.servers = append(r.servers, srv)
	r.resetState()
	return nil
}

// RemoveServer remove a server
func (r *RoundRobinWeight) RemoveServer(u *url.URL) error {
	r.mutex.Lock()

	e, index := r.findServerByURL(u)
	if e == nil {
		r.mutex.Unlock()
		return fmt.Errorf("server not found")
	}
	r.servers = append(r.servers[:index], r.servers[index+1:]...)
	r.resetState()
	r.mutex.Unlock()
	return nil
}

// Servers gets servers URL
func (r *RoundRobinWeight) Servers() []*url.URL {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	out := make([]*url.URL, len(r.servers))
	for i, srv := range r.servers {
		out[i] = srv.url
	}
	return out
}

// ServerWeight gets the server weight
func (r *RoundRobinWeight) ServerWeight(u *url.URL) (int, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if s, _ := r.findServerByURL(u); s != nil {
		return s.weight, true
	}
	return -1, false
}

func (r *RoundRobinWeight) resetState() {
	s := buildServerList(r.servers)
	r.serverList.Store(s)
}

func (r *RoundRobinWeight) findServerByURL(u *url.URL) (*server, int) {
	if len(r.servers) == 0 {
		return nil, -1
	}
	for i, s := range r.servers {
		if sameURL(u, s.url) {
			return s, i
		}
	}
	return nil, -1
}

// serverList 服务列表选择
type serverList struct {
	servers []*server
	r       *rand.Rand
}

func buildServerList(servers []*server) *serverList {
	s := &serverList{
		r: rand.New(newLockedSource()),
	}
	if len(servers) < 0 {
		return s
	}
	s.servers = make([]*server, 0, len(servers))
	for _, v := range servers {
		if v.weight <= 0 {
			panic(fmt.Errorf("url:%v weight:%d must be a positive integer", v.url, v.weight))
		}
		tmp := *v
		s.servers = append(s.servers, &tmp)
	}
	for i := 1; i < len(servers); i++ {
		s.servers[i].weight += s.servers[i-1].weight
	}
	return s
}

func (s *serverList) nextServer() *url.URL {
	n := len(s.servers)
	if n == 0 {
		return nil
	}
	val := s.r.Intn(s.servers[n-1].weight)
	li, ri := 0, n
	for li < ri {
		m := (li + ri) >> 1
		if s.servers[m].weight <= val {
			li = m + 1
		} else if s.servers[m].weight > val {
			ri = m
		}
	}
	return s.servers[li].url
}

type lockedSource struct {
	lk  sync.Mutex
	src rand.Source
}

func newLockedSource() rand.Source {
	return &lockedSource{
		src: rand.NewSource(time.Now().UnixNano()),
	}
}

func (r *lockedSource) Int63() (n int64) {
	r.lk.Lock()
	n = r.src.Int63()
	r.lk.Unlock()
	return
}

func (r *lockedSource) Seed(seed int64) {
	r.lk.Lock()
	r.src.Seed(seed)
	r.lk.Unlock()
}
