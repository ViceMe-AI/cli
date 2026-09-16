package s3publish

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	defaultConcurrency = 8
	defaultTimeout     = 60 * time.Second
	maxAttempts        = 3
)

func httpClientFor(proxyURL string, timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL == "" {
		// Never inherit process-wide HTTP(S)_PROXY: Global must stay direct
		// even when the CN region uses an explicit proxy in the same process.
		transport.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	} else {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid region proxy URL")
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}

func newS3Client(ctx context.Context, region Region, httpClient *http.Client) (*s3.Client, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(region.AccessKey, region.SecretKey, "")),
		config.WithHTTPClient(httpClient),
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		config.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
		config.WithRetryMaxAttempts(maxAttempts),
	)
	if err != nil {
		return nil, fmt.Errorf("%s storage client setup failed", region.Label)
	}
	return s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(region.Endpoint)
		options.UsePathStyle = true
		options.EndpointOptions.DisableHTTPS = strings.HasPrefix(region.Endpoint, "http://")
		options.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		options.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	}), nil
}
