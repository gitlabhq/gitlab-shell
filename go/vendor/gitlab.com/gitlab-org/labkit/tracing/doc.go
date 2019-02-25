/*
Package tracing is the primary entrypoint into LabKit's distributed tracing functionality.

(This documentation assumes some minimal knowledge of Distributed Tracing, and uses
tracing terminology without providing definitions. Please review
https://opentracing.io/docs/overview/what-is-tracing/ for an broad overview of distributed
tracing if you are not familiar with the technology)

Internally the `tracing` package relies on Opentracing, but avoids leaking this abstraction.
In theory, LabKit could replace Opentracing with another distributed tracing interface, such
as Zipkin or OpenCensus, without needing to make changes to the application (other than vendoring
in a new version of LabKit, of course).

This design decision is deliberate: the package should not leak the underlying tracing implementation.

The package provides three primary exports:

* `tracing.Initialize()` for initializing the global tracer using the `GITLAB_TRACING` environment variable.
* An HTTP Handler middleware, `tracing.Handler()`, for instrumenting incoming HTTP requests.
* An HTTP RoundTripper, `tracing.NewRoundTripper()` for instrumenting outbound HTTP requests to other services.

The provided example in `example_test.go` demonstrates usage of both the HTTP Middleware and the HTTP RoundTripper.

*Initializing the global tracer*

Opentracing makes use of a global tracer. Opentracing ships with a default NoOp tracer which does
nothing at all. This is always configured, meaning that, without initialization, Opentracing does nothing and
has a very low overhead.

LabKit's tracing is configured through an environment variable, `GITLAB_TRACING`. This environment variable contains
a "connection string"-like configuration, such as:

* `opentracing://jaeger?udp_endpoint=localhost:6831`
* `opentracing://datadog`
* `opentracing://lightstep`

The parameters for these connection-strings are implementation specific.

This configuration is identical to the one used to configure GitLab's ruby tracing libraries in the `Gitlab::Tracing`
package. Having a consistent configuration makes it easy to configure multiple processes at the same time. For example,
in GitLab Development Kit, tracing can be configured with a single environment variable, `GITLAB_TRACING=... gdk run`,
since `GITLAB_TRACING` will configure Workhorse (written in Go), Gitaly (written in Go) and GitLab's rails components,
using the same configuration.

*Compiling applications with Tracing support*

Go's Opentracing interface does not allow tracing implementations to be loaded dynamically; implementations need to be
compiled into the application. With LabKit, this is done conditionally, using build tags. Two build tags need to be
specified:

* `tracer_static` - this compiles in the static plugin registry support
* `tracer_static_[DRIVER_NAME]` - this compile in support for the given driver.

For example, to compile support for Jaeger, compile your Go app with `tracer_static,tracer_static_jaeger`

Note that multiple (or all) drivers can be compiled in alongside one another: using the tags:
`tracer_static,tracer_static_jaeger,tracer_static_lightstep,tracer_static_datadog`

If the `GITLAB_TRACING` environment variable references an unknown or unregistered driver, it will log a message
and continue without tracing. This is a deliberate decision: the risk of bringing down a cluster during a rollout
with a misconfigured tracer configuration is greater than the risk of an operator loosing some time because
their application was not compiled with the correct tracers.

*Using the HTTP Handler middleware to instrument incoming HTTP requests*

When an incoming HTTP request arrives on the server, it may already include Distributed Tracing headers,
propagated from an upstream service.

The tracing middleware will attempt to extract the tracing information from the headers (the exact headers used are
tracing implementation specific), set up a span and pass the information through the request context.

It is up to the Opentracing implementation to decide whether the span will be sent to the tracing infrastructure.
This will be implementation-specific, but generally relies on server load, sampler configuration, whether an
error occurred, whether certain spans took an anomalous amount of time, etc.

*Using the  HTTP RoundTripper to instrument outgoing HTTP requests*

The RoundTripper should be added to the HTTP client RoundTripper stack (see the example). When an outbound
HTTP request is sent from the HTTP client, the RoundTripper will determine whether there is an active span
and if so, will inject headers into the outgoing HTTP request identifying the span. The details of these
headers is implementation specific.

It is important to ensure that the context is passed into the outgoing request, using `req.WithContext(ctx)`
so that the correct span information can be injected into the request headers.

*Propagating tracing information to child processes*

Sometimes we want a trace to continue from a parent process to a spawned child process. For this,
the tracing package provides `tracing.NewEnvInjector()` and `tracing.ExtractFromEnv()`, for the
parent and child processes respectively.

NewEnvInjector() will configure a []string array of environment variables, ensuring they have the
correct tracing configuration and any trace and span identifiers. NewEnvInjector() should be called
in the child process and will extract the trace and span information from the environment.

Please review the examples in the godocs for details of how to implement both approaches.
*/
package tracing
