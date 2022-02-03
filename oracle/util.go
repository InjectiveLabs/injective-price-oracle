package oracle

import (
	"net/url"
	"path"
)

func urlJoin(baseURL string, segments ...string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		panic(err)
	}
	u.Path = path.Join(append([]string{u.Path}, segments...)...)
	return u.String()
}

func stripToken(reqURL *url.URL) *url.URL {
	v := reqURL.Query()
	v.Del("token")
	reqURL.RawQuery = v.Encode()
	return reqURL
}
