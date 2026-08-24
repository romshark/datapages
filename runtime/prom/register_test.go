package prom_test

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/runtime/prom"
)

// builtinCount is how many datapages metric families r carries.
// A counter that was never observed reports nothing, which is why this
// counts families rather than comparing against the number of collectors.
func builtinCount(t *testing.T, r *prometheus.Registry) int {
	t.Helper()
	mfs, err := r.Gather()
	require.NoError(t, err)
	n := 0
	for _, mf := range mfs {
		if strings.HasPrefix(mf.GetName(), "datapages_") {
			n++
		}
	}
	return n
}

// TestRegisterPerRegisterer covers two servers of one process, each given a
// registerer of its own. Both have to end up with the metrics: registering
// once per process would leave the second endpoint empty and say nothing.
func TestRegisterPerRegisterer(t *testing.T) {
	first, second := prometheus.NewRegistry(), prometheus.NewRegistry()
	require.NoError(t, prom.Register(first))
	require.NoError(t, prom.Register(second))

	got := builtinCount(t, first)
	require.NotZero(t, got, "the first registerer carries no metrics")
	require.Equal(t, got, builtinCount(t, second),
		"the second registerer carries fewer metrics than the first")
}

// TestRegisterTwiceOnOneRegisterer covers two servers sharing a registerer,
// which is what they get by default. The second registration is a no-op
// instead of the duplicate MustRegister panics on.
func TestRegisterTwiceOnOneRegisterer(t *testing.T) {
	r := prometheus.NewRegistry()
	require.NoError(t, prom.Register(r))
	require.NoError(t, prom.Register(r))
	require.NotZero(t, builtinCount(t, r))
}

// TestRegisterExtraCollectors covers the user-defined collectors,
// which follow the same rule: a second server passing the same one must not fail.
func TestRegisterExtraCollectors(t *testing.T) {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "user_defined_total", Help: "user defined",
	})
	c.Inc()

	r := prometheus.NewRegistry()
	require.NoError(t, prom.Register(r, c))
	require.NoError(t, prom.Register(r, c))

	mfs, err := r.Gather()
	require.NoError(t, err)
	var found bool
	for _, mf := range mfs {
		if mf.GetName() == "user_defined_total" {
			found = true
		}
	}
	require.True(t, found, "the user collector was not registered")
}

// TestRegisterReportsAConflict covers a collector that clashes with one the
// registerer holds under the same name but a different type.
func TestRegisterReportsAConflict(t *testing.T) {
	r := prometheus.NewRegistry()
	require.NoError(t, r.Register(prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "clashing", Help: "a gauge",
	})))
	err := prom.Register(r, prometheus.NewCounter(prometheus.CounterOpts{
		Name: "clashing", Help: "a counter",
	}))
	require.Error(t, err, "a real conflict was accepted")
}
