package main

import (
	"errors"
	"fmt"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/prometheus/client_golang/prometheus"
)

// registerAdvisoryLockMetrics exposes the framework's advisory_lock_count and
// advisory_lock_duration metrics on /metrics. The framework records them on
// every advisory lock but never registers them, so without this call they
// never reach Prometheus (API-OBS-10).
//
// The framework registers with prometheus.MustRegister, which panics. A
// framework release that starts registering them itself would make this a
// duplicate, so an AlreadyRegisteredError counts as success and any other
// panic becomes an error.
func registerAdvisoryLockMetrics() (err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if e, ok := r.(error); ok {
			var already prometheus.AlreadyRegisteredError
			if errors.As(e, &already) {
				err = nil
				return
			}
		}
		err = fmt.Errorf("register advisory lock metrics: %v", r)
	}()
	db.RegisterAdvisoryLockMetrics()
	return nil
}
