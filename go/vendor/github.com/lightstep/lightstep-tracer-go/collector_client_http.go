package lightstep

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/lightstep/lightstep-tracer-go/collectorpb"
)

var (
	acceptHeader      = http.CanonicalHeaderKey("Accept")
	contentTypeHeader = http.CanonicalHeaderKey("Content-Type")
)

const (
	collectorHTTPMethod = "POST"
	collectorHTTPPath   = "/api/v2/reports"
	protoContentType    = "application/octet-stream"
)

// grpcCollectorClient specifies how to send reports back to a LightStep
// collector via grpc.
type httpCollectorClient struct {
	// auth and runtime information
	reporterID  uint64
	accessToken string // accessToken is the access token used for explicit trace collection requests.
	attributes  map[string]string

	reportTimeout   time.Duration
	reportingPeriod time.Duration

	// Remote service that will receive reports.
	url    *url.URL
	client *http.Client

	// converters
	converter *protoConverter
}

type transportCloser struct {
	*http.Transport
}

func (closer transportCloser) Close() error {
	closer.CloseIdleConnections()
	return nil
}

func newHTTPCollectorClient(
	opts Options,
	reporterID uint64,
	attributes map[string]string,
) (*httpCollectorClient, error) {
	url, err := url.Parse(opts.Collector.URL())
	if err != nil {
		fmt.Println("collector config does not produce valid url", err)
		return nil, err
	}
	url.Path = collectorHTTPPath

	return &httpCollectorClient{
		reporterID:      reporterID,
		accessToken:     opts.AccessToken,
		attributes:      attributes,
		reportTimeout:   opts.ReportTimeout,
		reportingPeriod: opts.ReportingPeriod,
		url:             url,
		converter:       newProtoConverter(opts),
	}, nil
}

func (client *httpCollectorClient) ConnectClient() (Connection, error) {
	// Use a transport independent from http.DefaultTransport to provide sane
	// defaults that make sense in the context of the lightstep client. The
	// differences are mostly on setting timeouts based on the report timeout
	// and period.
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   client.reportTimeout / 2,
			DualStack: true,
		}).DialContext,
		// The collector responses are very small, there is no point asking for
		// a compressed payload, explicitly disabling it.
		DisableCompression:     true,
		IdleConnTimeout:        2 * client.reportingPeriod,
		TLSHandshakeTimeout:    client.reportTimeout / 2,
		ResponseHeaderTimeout:  client.reportTimeout,
		ExpectContinueTimeout:  client.reportTimeout,
		MaxResponseHeaderBytes: 64 * 1024, // 64 KB, just a safeguard
	}

	client.client = &http.Client{
		Transport: transport,
		Timeout:   client.reportTimeout,
	}

	return transportCloser{transport}, nil
}

func (client *httpCollectorClient) ShouldReconnect() bool {
	// http.Transport will handle connection reuse under the hood
	return false
}

func (client *httpCollectorClient) Report(context context.Context, req reportRequest) (collectorResponse, error) {
	if req.httpRequest == nil {
		return nil, fmt.Errorf("httpRequest cannot be null")
	}

	httpResponse, err := client.client.Do(req.httpRequest.WithContext(context))
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()

	response, err := client.toResponse(httpResponse)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (client *httpCollectorClient) Translate(ctx context.Context, buffer *reportBuffer) (reportRequest, error) {

	httpRequest, err := client.toRequest(ctx, buffer)
	if err != nil {
		return reportRequest{}, err
	}
	return reportRequest{
		httpRequest: httpRequest,
	}, nil
}

func (client *httpCollectorClient) toRequest(
	context context.Context,
	buffer *reportBuffer,
) (*http.Request, error) {
	protoRequest := client.converter.toReportRequest(
		client.reporterID,
		client.attributes,
		client.accessToken,
		buffer,
	)

	buf, err := proto.Marshal(protoRequest)
	if err != nil {
		return nil, err
	}

	requestBody := bytes.NewReader(buf)

	request, err := http.NewRequest(collectorHTTPMethod, client.url.String(), requestBody)
	if err != nil {
		return nil, err
	}
	request = request.WithContext(context)
	request.Header.Set(contentTypeHeader, protoContentType)
	request.Header.Set(acceptHeader, protoContentType)

	return request, nil
}

func (client *httpCollectorClient) toResponse(response *http.Response) (collectorResponse, error) {
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code (%d) is not ok", response.StatusCode)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	protoResponse := &collectorpb.ReportResponse{}
	if err := proto.Unmarshal(body, protoResponse); err != nil {
		return nil, err
	}

	return protoResponse, nil
}
