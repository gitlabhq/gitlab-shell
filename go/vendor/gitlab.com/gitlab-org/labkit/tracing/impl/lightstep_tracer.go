// +build tracer_static,tracer_static_lightstep

package impl

import (
	"context"
	"fmt"
	"io"
	"log"

	lightstep "github.com/lightstep/lightstep-tracer-go"
	opentracing "github.com/opentracing/opentracing-go"
)

type lightstepCloser struct {
	tracer lightstep.Tracer
}

func (c *lightstepCloser) Close() error {
	lightstep.Close(context.Background(), c.tracer)
	return nil
}

var lightstepConfigMapper = map[string]func(traceCfg *lightstep.Options, value string) error{
	"ServiceName": func(options *lightstep.Options, value string) error {
		options.Tags[lightstep.ComponentNameKey] = value
		return nil
	},
	"access_token": func(options *lightstep.Options, value string) error {
		options.AccessToken = value
		return nil
	},
}

func lightstepTracerFactory(config map[string]string) (opentracing.Tracer, io.Closer, error) {
	options := lightstep.Options{
		Tags: map[string]interface{}{},
	}

	// Convert the configuration map into a jaeger configuration
	for k, v := range config {
		mapper := lightstepConfigMapper[k]
		if k == keyStrictConnectionParsing {
			continue
		}

		if mapper != nil {
			err := mapper(&options, v)
			if err != nil {
				return nil, nil, err
			}
		} else {
			if config[keyStrictConnectionParsing] != "" {
				return nil, nil, fmt.Errorf("lightstep tracer: invalid option: %s", k)
			}

			log.Printf("lightstep tracer: warning: ignoring unknown configuration option: %s", k)
		}
	}

	tracer := lightstep.NewTracer(options)
	if tracer == nil {
		return nil, nil, fmt.Errorf("lightstep tracer: unable to create tracer, review log messages")
	}

	return tracer, &lightstepCloser{tracer}, nil
}

func init() {
	registerTracer("lightstep", lightstepTracerFactory)
}
