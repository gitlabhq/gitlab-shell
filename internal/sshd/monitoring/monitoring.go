package monitoring

import (
	"crypto/tls"
	"net"

	"github.com/prometheus/client_golang/prometheus"

	"gitlab.com/gitlab-org/labkit/log"
	"gitlab.com/gitlab-org/labkit/monitoring"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshd"
)

var tlsVersions = map[string]uint16{
	"":       0, // Default value in tls.Config
	"tls1.0": tls.VersionTLS10,
	"tls1.1": tls.VersionTLS11,
	"tls1.2": tls.VersionTLS12,
	"tls1.3": tls.VersionTLS13,
}

type WebServer struct {
	ListenerConfigs []config.ListenerConfig

	listeners []net.Listener
}

func (w *WebServer) Start(version, buildTime string, sshdServer *sshd.Server) error {
	var prometheusRegisterer prometheus.Registerer
	for _, cfg := range w.ListenerConfigs {
		if cfg.Addr == "" {
			continue
		}

		listener, err := w.newListener(cfg)
		if err != nil {
			return err
		}

		monitoringOpts := []monitoring.Option{
			monitoring.WithListener(listener),
			monitoring.WithServeMux(sshdServer.MonitoringServeMux()),
		}

		// We have to cache and reuse Prometheus registerer in order to avoid
		// duplicate metrics collector registration attempted by Labkit
		if prometheusRegisterer == nil {
			monitoringOpts = append(monitoringOpts, monitoring.WithBuildInformation(version, buildTime))
		} else {
			monitoringOpts = append(monitoringOpts, monitoring.WithPrometheusRegisterer(prometheusRegisterer))
		}

		go func() {
			if err := monitoring.Start(monitoringOpts...); err != nil {
				log.WithError(err).Fatal("monitoring service raised an error")
			}
		}()

		prometheusRegisterer = prometheus.DefaultRegisterer
		w.listeners = append(w.listeners, listener)
	}

	return nil
}

func (w *WebServer) newListener(cfg config.ListenerConfig) (net.Listener, error) {
	if cfg.Tls == nil {
		log.WithFields(log.Fields{"address": cfg.Addr}).Infof("Running monitoring server")

		return net.Listen("tcp", cfg.Addr)
	}

	cert, err := tls.LoadX509KeyPair(cfg.Tls.Certificate, cfg.Tls.Key)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		MinVersion:   tlsVersions[cfg.Tls.MinVersion],
		MaxVersion:   tlsVersions[cfg.Tls.MaxVersion],
		Certificates: []tls.Certificate{cert},
	}

	log.WithFields(log.Fields{"address": cfg.Addr}).Infof("Running monitoring server with tls")

	return tls.Listen("tcp", cfg.Addr, tlsConfig)
}
