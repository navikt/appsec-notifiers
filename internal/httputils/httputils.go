package httputils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const UserAgent = "NAV IT McBotFace"

func GQLRequest(ctx context.Context, rawUrl, body string, headers http.Header) (io.ReadCloser, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewBuffer([]byte(body)))
	if err != nil {
		return nil, err
	}
	req.Header = headers
	client := http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status code %d from %q: %v", res.StatusCode, rawUrl, res)
	}
	return res.Body, nil
}

func GetRequest(url string) ([]byte, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return resBody, nil
}
