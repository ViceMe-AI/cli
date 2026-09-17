package config

import "net/url"

// APIStateBaseURL preserves the storage namespace used before the Gateway
// cutover. Its input is a validated, canonical API base URL. It is only for
// local credentials, bindings and recovery records; never send requests here.
// Custom endpoints keep their existing identity, including their path/port.
func APIStateBaseURL(baseURL string) string {
	switch baseURL {
	case "https://viceme.cn/api":
		return "https://api.viceme.cn"
	case "https://viceme.ai/api":
		return "https://api.viceme.ai"
	default:
		return baseURL
	}
}

// APIStateOrigin is for stores whose established key omits the endpoint path.
func APIStateOrigin(raw string) (string, error) {
	baseURL, err := NormalizeAPIBaseURL(raw)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(APIStateBaseURL(baseURL))
	if err != nil {
		return "", err
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// EquivalentAPIBaseURLs allows only the two known official endpoint cutovers.
// It does not merge markets, custom paths or non-default ports.
func EquivalentAPIBaseURLs(left, right string) bool {
	a, errA := NormalizeAPIBaseURL(left)
	b, errB := NormalizeAPIBaseURL(right)
	return errA == nil && errB == nil && a == b
}
