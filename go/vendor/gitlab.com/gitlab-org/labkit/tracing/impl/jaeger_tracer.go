// +build tracer_static,tracer_static_jaeger

package impl

import (
	"fmt"
	"io"
	"log"
	"strconv"

	opentracing "github.com/opentracing/opentracing-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
)

type traceConfigMapper func(traceCfg *jaegercfg.Configuration, value string) ([]jaegercfg.Option, error)

var configMapper = map[string]traceConfigMapper{
	"ServiceName": func(traceCfg *jaegercfg.Configuration, value string) ([]jaegercfg.Option, error) {
		traceCfg.ServiceName = value
		return nil, nil
	},
	"debug": func(traceCfg *jaegercfg.Configuration, value string) ([]jaegercfg.Option, error) {
		return []jaegercfg.Option{jaegercfg.Logger(jaegerlog.StdLogger)}, nil
	},
	"sampler": func(traceCfg *jaegercfg.Configuration, value string) ([]jaegercfg.Option, error) {
		if traceCfg.Sampler == nil {
			traceCfg.Sampler = &jaegercfg.SamplerConfig{}
		}

		traceCfg.Sampler.Type = value
		return nil, nil
	},
	"sampler_param": func(traceCfg *jaegercfg.Configuration, value string) ([]jaegercfg.Option, error) {
		if traceCfg.Sampler == nil {
			traceCfg.Sampler = &jaegercfg.SamplerConfig{}
		}
		valuef, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("jaeger tracer: sampler_param must be a float")
		}

		traceCfg.Sampler.Param = valuef
		return nil, nil
	},
	"http_endpoint": func(traceCfg *jaegercfg.Configuration, value string) ([]jaegercfg.Option, error) {
		if traceCfg.Reporter == nil {
			traceCfg.Reporter = &jaegercfg.ReporterConfig{}
		}
		traceCfg.Reporter.CollectorEndpoint = value
		return nil, nil
	},
	"udp_endpoint": func(traceCfg *jaegercfg.Configuration, value string) ([]jaegercfg.Option, error) {
		if traceCfg.Reporter == nil {
			traceCfg.Reporter = &jaegercfg.ReporterConfig{}
		}
		traceCfg.Reporter.LocalAgentHostPort = value
		return nil, nil
	},
}

func jaegerTracerFactory(config map[string]string) (opentracing.Tracer, io.Closer, error) {
	traceCfg, err := jaegercfg.FromEnv()
	if err != nil {
		return nil, nil, err
	}
	options := []jaegercfg.Option{}

	// Convert the configuration map into a jaeger configuration
	for k, v := range config {
		mapper := configMapper[k]
		if k == keyStrictConnectionParsing {
			continue
		}

		if mapper != nil {
			o, err := mapper(traceCfg, v)
			if err != nil {
				return nil, nil, err
			}
			options = append(options, o...)
		} else {
			if config[keyStrictConnectionParsing] != "" {
				return nil, nil, fmt.Errorf("jaeger tracer: invalid option: %s", k)
			}

			log.Printf("jaeger tracer: warning: ignoring unknown configuration option: %s", k)
		}
	}

	return traceCfg.NewTracer(options...)
}

func init() {
	registerTracer("jaeger", jaegerTracerFactory)
}
