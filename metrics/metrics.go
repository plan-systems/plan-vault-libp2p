package metrics

import (
	"context"
	"fmt"
	"net/http"

	"github.com/apex/log"
	gometrics "github.com/armon/go-metrics"
	pmetrics "github.com/armon/go-metrics/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Run(ctx context.Context, cfg *Config) {
	if cfg.DisableDebug {
		return
	}

	sink, _ := pmetrics.NewPrometheusSink()
	gometrics.NewGlobal(gometrics.DefaultConfig("plan-vault"), sink)

	srv := &http.Server{
		Addr: fmt.Sprintf("%v:%v", cfg.DebugAddr, cfg.DebugPort),
	}

	http.Handle("/metrics", promhttp.Handler())

	go func() { <-ctx.Done(); srv.Shutdown(context.Background()) }()

	go func() {
		err := srv.ListenAndServe()
		if err != nil {
			log.Fatal(err.Error())
		}
	}()
}
