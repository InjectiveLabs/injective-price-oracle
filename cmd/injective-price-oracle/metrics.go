package main

import (
	"os"
	"time"

	log "github.com/InjectiveLabs/suplog"
	"github.com/xlab/closer"

	// DEBUG: do not enable in production
	// _ "net/http/pprof"

	"github.com/InjectiveLabs/metrics"
)

// startMetricsGathering initializes metric reporting client,
// if not globally disabled by the config.
func startMetricsGathering(
	statsdPrefix *string,
	statsdAddr *string,
	statsdAgent *string,
	statsdStuckDur *string,
	statsdMocking *string,
	statsdDisabled *string,
) {
	if toBool(*statsdDisabled) {
		// initializes statsd client with a mock one with no-op enabled
		metrics.Disable()
		return
	}

	go func() {
		for {
			hostname, _ := os.Hostname()
			err := metrics.Init(*statsdAddr, checkStatsdPrefix(*statsdPrefix), &metrics.StatterConfig{
				EnvName:              *envName,
				Agent:                *statsdAgent,
				HostName:             hostname,
				StuckFunctionTimeout: duration(*statsdStuckDur, 30*time.Minute),
				MockingEnabled:       toBool(*statsdMocking) || *envName == "local",
			})
			if err != nil {
				log.WithError(err).Warningln("metrics init failed, will retry in 1 min")
				time.Sleep(time.Minute)
				continue
			}
			break
		}

		closer.Bind(func() {
			metrics.Close()
		})
	}()

}
