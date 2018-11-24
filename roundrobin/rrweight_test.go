package roundrobin

import (
	"math"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"

	"github.com/qutoutiao/oxy/forward"
	"github.com/qutoutiao/oxy/testutils"
	"github.com/qutoutiao/oxy/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundRobinWeightNoServers(t *testing.T) {
	fwd, err := forward.New()
	require.NoError(t, err)

	lb, err := NewRoundRobinWeight(fwd)
	require.NoError(t, err)

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	re, _, err := testutils.Get(proxy.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, re.StatusCode)
}

func TestRoundRobinWeightRemoveBadServer(t *testing.T) {
	lb, err := NewRoundRobinWeight(nil)
	require.NoError(t, err)

	assert.Error(t, lb.RemoveServer(testutils.ParseURI("http://google.com")))
}

func TestRoundRobinWeightCustomErrHandler(t *testing.T) {
	errHandler := utils.ErrorHandlerFunc(func(w http.ResponseWriter, req *http.Request, err error) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte(http.StatusText(http.StatusTeapot)))
	})

	fwd, err := forward.New()
	require.NoError(t, err)

	lb, err := NewRoundRobinWeight(fwd, RoundRobinWeightErrorHandler(errHandler))
	require.NoError(t, err)

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	re, _, err := testutils.Get(proxy.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTeapot, re.StatusCode)
}

func TestRoundRobinWeightOneServer(t *testing.T) {
	a := testutils.NewResponder("a")
	defer a.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	lb, err := NewRoundRobinWeight(fwd)
	require.NoError(t, err)

	require.NoError(t, lb.UpsertServer(testutils.ParseURI(a.URL)))

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	assert.Equal(t, []string{"a", "a", "a"}, seq(t, proxy.URL, 3))
}

func TestRoundRobinWeightSimple(t *testing.T) {
	a := testutils.NewResponder("a")
	defer a.Close()

	b := testutils.NewResponder("b")
	defer b.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	lb, err := NewRoundRobinWeight(fwd)
	require.NoError(t, err)

	require.NoError(t, lb.UpsertServer(testutils.ParseURI(a.URL)))
	require.NoError(t, lb.UpsertServer(testutils.ParseURI(b.URL)))

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	list := seq(t, proxy.URL, 500)
	count := make(map[string]int)
	for _, v := range list {
		count[v]++
	}
	assert.Equal(t, 2, len(count))
	checkDeviation(t, count["a"], count["b"], 0.05)
}

func TestRoundRobinWeightRemoveServer(t *testing.T) {
	a := testutils.NewResponder("a")
	defer a.Close()

	b := testutils.NewResponder("b")
	defer b.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	lb, err := NewRoundRobinWeight(fwd)
	require.NoError(t, err)

	require.NoError(t, lb.UpsertServer(testutils.ParseURI(a.URL)))
	require.NoError(t, lb.UpsertServer(testutils.ParseURI(b.URL)))

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	list := seq(t, proxy.URL, 500)
	count := make(map[string]int)
	for _, v := range list {
		count[v]++
	}
	assert.Equal(t, 2, len(count))
	checkDeviation(t, count["a"], count["b"], 0.05)

	err = lb.RemoveServer(testutils.ParseURI(a.URL))
	require.NoError(t, err)

	assert.Equal(t, []string{"b", "b", "b"}, seq(t, proxy.URL, 3))
}

func TestRoundRobinWeightUpsertSame(t *testing.T) {
	a := testutils.NewResponder("a")
	defer a.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	lb, err := NewRoundRobinWeight(fwd)
	require.NoError(t, err)

	require.NoError(t, lb.UpsertServer(testutils.ParseURI(a.URL)))
	require.NoError(t, lb.UpsertServer(testutils.ParseURI(a.URL)))

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	assert.Equal(t, []string{"a", "a", "a"}, seq(t, proxy.URL, 3))
}

func TestRoundRobinWeightUpsertWeight(t *testing.T) {
	a := testutils.NewResponder("a")
	defer a.Close()

	b := testutils.NewResponder("b")
	defer b.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	lb, err := NewRoundRobinWeight(fwd)
	require.NoError(t, err)

	require.NoError(t, lb.UpsertServer(testutils.ParseURI(a.URL)))
	require.NoError(t, lb.UpsertServer(testutils.ParseURI(b.URL)))

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	list := seq(t, proxy.URL, 500)
	count := make(map[string]int)
	for _, v := range list {
		count[v]++
	}
	assert.Equal(t, 2, len(count))
	checkDeviation(t, count["a"], count["b"], 0.05)

	assert.NoError(t, lb.UpsertServer(testutils.ParseURI(b.URL), Weight(3)))

	list = seq(t, proxy.URL, 500)
	count = make(map[string]int)
	for _, v := range list {
		count[v]++
	}
	assert.Equal(t, 2, len(count))
	checkDeviation(t, count["a"], count["b"], 0.55, 0.45)
}

func TestRoundRobinWeightWeighted(t *testing.T) {
	require.NoError(t, SetDefaultWeight(1))
	defer SetDefaultWeight(1)

	a := testutils.NewResponder("a")
	defer a.Close()

	b := testutils.NewResponder("b")
	defer b.Close()

	z := testutils.NewResponder("z")
	defer z.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	lb, err := NewRoundRobinWeight(fwd)
	require.NoError(t, err)

	require.NoError(t, lb.UpsertServer(testutils.ParseURI(a.URL), Weight(3)))
	require.NoError(t, lb.UpsertServer(testutils.ParseURI(b.URL), Weight(2)))
	require.NoError(t, lb.UpsertServer(testutils.ParseURI(z.URL), Weight(0)))

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	list := seq(t, proxy.URL, 5000)
	count := make(map[string]int)
	for _, v := range list {
		count[v]++
	}
	assert.Equal(t, 3, len(count))
	checkDeviation(t, count["a"], count["b"], 0.25, 0.15)
	checkDeviation(t, count["a"], count["z"], 0.55, 0.45)
	checkDeviation(t, count["b"], count["z"], 0.35, 0.25)

	// assert.Equal(t, []string{"a", "a", "b", "a", "b", "a"}, seq(t, proxy.URL, 6))

	w, ok := lb.ServerWeight(testutils.ParseURI(a.URL))
	assert.Equal(t, 3, w)
	assert.Equal(t, true, ok)

	w, ok = lb.ServerWeight(testutils.ParseURI(b.URL))
	assert.Equal(t, 2, w)
	assert.Equal(t, true, ok)

	w, ok = lb.ServerWeight(testutils.ParseURI(z.URL))
	assert.Equal(t, 1, w)
	assert.Equal(t, true, ok)

	w, ok = lb.ServerWeight(testutils.ParseURI("http://caramba:4000"))
	assert.Equal(t, -1, w)
	assert.Equal(t, false, ok)
}

func TestRoundRobinWeightRace(t *testing.T) {
	a := testutils.NewResponder("a")
	defer a.Close()

	b := testutils.NewResponder("b")
	defer b.Close()

	z := testutils.NewResponder("z")
	defer z.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	lb, err := NewRoundRobinWeight(fwd)
	require.NoError(t, err)

	proxy := httptest.NewServer(lb)
	defer proxy.Close()
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				require.NoError(t, lb.UpsertServer(testutils.ParseURI(a.URL), Weight(3)))
				require.NoError(t, lb.UpsertServer(testutils.ParseURI(b.URL), Weight(2)))
				require.NoError(t, lb.UpsertServer(testutils.ParseURI(z.URL), Weight(0)))

				lb.RemoveServer(testutils.ParseURI(a.URL))

				lb.NextServer()
				// require.NoError(t, err)
			}
		}()
	}
	wg.Wait()

}

func checkDeviation(t *testing.T, a, b int, limit ...float64) {
	_, f, l, _ := runtime.Caller(1)
	switch len(limit) {
	case 1:
		abs := math.Abs(float64(a-b)) / float64(a+b)
		if abs > limit[0] {
			t.Fatalf("%s:%d a:%d, b:%d, abs:%f more than %f", f, l, a, b, abs, limit[0])
		}
	case 2:
		abs := math.Abs(float64(a-b)) / float64(a+b)
		if abs > limit[0] || abs < limit[1] {
			t.Fatalf("%s:%d a:%d, b:%d, abs:%f not on[%f,%f] ", f, l, a, b, abs, limit[1], limit[0])
		}
	default:
		t.Fatalf("%s:%d limit len err:%d", f, l, len(limit))
	}

}
