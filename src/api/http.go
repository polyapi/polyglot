package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/polyapi/polyglot/src/config"
	"github.com/polyapi/polyglot/src/version"
)

const versionHeader = "x-poly-api-version"

// Options are tunables for HTTPClient.
type Options struct {
	Timeout    time.Duration
	Retry      RetryPolicy
	APIVersion string
}

func defaultOptions() Options {
	return Options{
		Timeout: 30 * time.Second,
		Retry:   DefaultRetry(),
	}
}

// HTTPClient is the live PolyAPI HTTP client. The API key lives only in request headers.
type HTTPClient struct {
	inner  *http.Client
	base   *url.URL
	retry  RetryPolicy
	header http.Header
}

func (c *HTTPClient) String() string {
	return fmt.Sprintf("HTTPClient{base:%s}", c.base)
}

func (c *HTTPClient) GoString() string { return c.String() }

// NewHTTPClient builds a client with Bearer auth and x-poly-api-version.
func NewHTTPClient(baseURL, apiKey string, options Options) (*HTTPClient, error) {
	if options.Timeout == 0 {
		options.Timeout = 30 * time.Second
	}
	if options.Retry.Max429 == 0 && options.Retry.MaxTransient == 0 && options.Retry.BaseDelay == 0 {
		options.Retry = DefaultRetry()
	}
	base, err := normalizeBase(baseURL)
	if err != nil {
		return nil, err
	}
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+apiKey)
	header.Set("User-Agent", "polyapi/"+version.Version)
	if options.APIVersion != "" {
		header.Set(versionHeader, options.APIVersion)
	}
	return &HTTPClient{
		inner:  &http.Client{Timeout: options.Timeout},
		base:   base,
		retry:  options.Retry,
		header: header,
	}, nil
}

// FromConfig builds an HTTPClient from resolved credentials.
func FromConfig(cfg config.Resolved) (*HTTPClient, error) {
	url, key, err := cfg.RequireCredentials()
	if err != nil {
		return nil, configErr(err)
	}
	opts := defaultOptions()
	if cfg.APIVersion != "" {
		opts.APIVersion = cfg.APIVersion
	}
	return NewHTTPClient(url, key, opts)
}

func (c *HTTPClient) BaseURL() string { return c.base.String() }

// Auth is GET /auth — tenant, environment, permission flags.
func (c *HTTPClient) Auth() (AuthData, error) {
	value, err := c.send(http.MethodGet, "auth", "", nil, nil)
	if err != nil {
		return AuthData{}, err
	}
	if value == nil {
		return AuthData{}, unexpectedList(c.base.JoinPath("auth").String())
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return AuthData{}, jsonErr(err)
	}
	var auth AuthData
	if err := json.Unmarshal(raw, &auth); err != nil {
		return AuthData{}, jsonErr(err)
	}
	return auth, nil
}

// ListAll walks page while the payload looks paginated.
func (c *HTTPClient) ListAll(resource string) ([]any, error) {
	return c.ListAllQuery(resource, nil)
}

// ListAllQuery is ListAll with extra query parameters (page is merged in).
func (c *HTTPClient) ListAllQuery(resource string, query [][2]string) ([]any, error) {
	var all []any
	var page *uint64
	for i := 0; i < 100; i++ {
		q := append([][2]string{}, query...)
		if page != nil {
			q = append(q, [2]string{"page", fmt.Sprintf("%d", *page)})
		}
		items, raw, err := c.listRaw(resource, q)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if raw == nil {
			break
		}
		next, ok := NextPage(raw)
		if !ok {
			break
		}
		page = &next
	}
	return all, nil
}

func (c *HTTPClient) List(resource string) ([]any, error) {
	items, _, err := c.listRaw(resource, nil)
	return items, err
}

func (c *HTTPClient) Get(resource, id string) (any, error) {
	return c.GetQuery(resource, id, nil)
}

// GetQuery is GET resource/id with query parameters.
func (c *HTTPClient) GetQuery(resource, id string, query [][2]string) (any, error) {
	value, err := c.sendQuery(http.MethodGet, resource, id, query, nil, nil)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, statusErr("GET", resource, 204, "")
	}
	return value, nil
}

// PostAction POSTs to resource/id/action (for example functions/server/{id}/execute).
func (c *HTTPClient) PostAction(resource, id, action string, payload any) (any, error) {
	return c.PostActionHeaders(resource, id, action, payload, nil)
}

// PostActionHeaders is PostAction with extra headers (for example x-otp).
func (c *HTTPClient) PostActionHeaders(resource, id, action string, payload any, headers map[string]string) (any, error) {
	return c.send(http.MethodPost, resource, joinID(id, action), payload, headers)
}

// DeleteAction DELETEs resource/id/action (for example functions/server/{id}/logs).
func (c *HTTPClient) DeleteAction(resource, id, action string) error {
	_, err := c.send(http.MethodDelete, resource, joinID(id, action), nil, nil)
	return err
}

// WithTimeout returns a shallow copy whose HTTP timeout is d.
func (c *HTTPClient) WithTimeout(d time.Duration) *HTTPClient {
	if c == nil {
		return nil
	}
	out := *c
	inner := *c.inner
	inner.Timeout = d
	out.inner = &inner
	return &out
}

func (c *HTTPClient) Create(resource string, payload any) (any, error) {
	return c.CreateHeaders(resource, payload, nil)
}

// CreateHeaders is Create with extra headers (for example x-otp).
func (c *HTTPClient) CreateHeaders(resource string, payload any, headers map[string]string) (any, error) {
	value, err := c.send(http.MethodPost, resource, "", payload, headers)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, statusErr("POST", resource, 204, "")
	}
	return value, nil
}

// Put is PUT resource with a JSON body (upserts such as functions/api).
func (c *HTTPClient) Put(resource string, payload any) (any, error) {
	value, err := c.send(http.MethodPut, resource, "", payload, nil)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, statusErr("PUT", resource, 204, "")
	}
	return value, nil
}

// PostQuery POSTs JSON to resource with query parameters. An empty 2xx body is success (nil, nil).
func (c *HTTPClient) PostQuery(resource string, query [][2]string, payload any) (any, error) {
	return c.sendQuery(http.MethodPost, resource, "", query, payload, nil)
}

// PostText POSTs a raw text/plain body (OpenAPI documents to specification-input/oas).
func (c *HTTPClient) PostText(resource string, query [][2]string, text string) (any, error) {
	return c.sendQuery(http.MethodPost, resource, "", query, rawBody{contentType: "text/plain; charset=utf-8", data: []byte(text)}, nil)
}

func (c *HTTPClient) Update(resource, id string, payload any, headers map[string]string) (any, error) {
	value, err := c.send(http.MethodPatch, resource, id, payload, headers)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, statusErr("PATCH", resource, 204, "")
	}
	return value, nil
}

func (c *HTTPClient) Delete(resource, id string, headers map[string]string) error {
	_, err := c.send(http.MethodDelete, resource, id, nil, headers)
	return err
}

func (c *HTTPClient) listRaw(resource string, query [][2]string) ([]any, any, error) {
	u, err := c.resourceURL(resource, "", query)
	if err != nil {
		return nil, nil, err
	}
	value, err := c.sendURL(http.MethodGet, u, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if value == nil {
		return nil, nil, nil
	}
	items, err := UnwrapList(u.String(), value)
	if err != nil {
		return nil, nil, err
	}
	return items, value, nil
}

func (c *HTTPClient) send(method, resource, id string, body any, extra map[string]string) (any, error) {
	return c.sendQuery(method, resource, id, nil, body, extra)
}

func (c *HTTPClient) sendQuery(method, resource, id string, query [][2]string, body any, extra map[string]string) (any, error) {
	u, err := c.resourceURL(resource, id, query)
	if err != nil {
		return nil, err
	}
	return c.sendURL(method, u, body, extra)
}

func joinID(id, action string) string {
	id = strings.Trim(id, "/")
	action = strings.Trim(action, "/")
	if id == "" {
		return action
	}
	if action == "" {
		return id
	}
	return id + "/" + action
}

type rawBody struct {
	contentType string
	data        []byte
}

func encodeBody(body any) (io.Reader, string, error) {
	if body == nil {
		return nil, "", nil
	}
	if raw, ok := body.(rawBody); ok {
		return bytes.NewReader(raw.data), raw.contentType, nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, "", jsonErr(err)
	}
	return bytes.NewReader(data), "application/json", nil
}

func (c *HTTPClient) sendURL(method string, u *url.URL, body any, extra map[string]string) (any, error) {
	beginRequestWait()
	defer endRequestWait()
	var attempts429, attemptsTransient uint
	for {
		payload, contentType, err := encodeBody(body)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequest(method, u.String(), payload)
		if err != nil {
			return nil, networkErr(err.Error())
		}
		for k, vals := range c.header {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		for k, v := range extra {
			req.Header.Set(k, v)
		}
		resp, err := c.inner.Do(req)
		if err != nil {
			if attemptsTransient < c.retry.MaxTransient {
				c.retry.sleep(c.retry.delayFor(attemptsTransient, 0))
				attemptsTransient++
				continue
			}
			return nil, networkErr(err.Error())
		}
		status := resp.StatusCode
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		if status == http.StatusTooManyRequests && attempts429 < c.retry.Max429 {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			c.retry.sleep(c.retry.delayFor(attempts429, retryAfter))
			attempts429++
			continue
		}
		if isTransientStatus(status) && method == http.MethodGet && attemptsTransient < c.retry.MaxTransient {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			c.retry.sleep(c.retry.delayFor(attemptsTransient, 0))
			attemptsTransient++
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		text := string(raw)
		if status == http.StatusNoContent {
			return nil, nil
		}
		if status < 200 || status >= 300 {
			return nil, FromStatus(method, u.String(), status, truncateBody(text))
		}
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return strings.TrimSpace(text), nil
		}
		return value, nil
	}
}

func (c *HTTPClient) resourceURL(resource, id string, query [][2]string) (*url.URL, error) {
	if !IsSafeResource(resource) {
		return nil, invalidResource(resource)
	}
	rel := strings.TrimLeft(resource, "/")
	u, err := url.Parse(c.base.String())
	if err != nil {
		return nil, invalidResource(err.Error())
	}
	joined, err := url.JoinPath(u.Path, rel)
	if err != nil {
		return nil, invalidResource(err.Error())
	}
	u.Path = joined
	if id != "" {
		u.Path, err = url.JoinPath(u.Path, id)
		if err != nil {
			return nil, invalidResource(resource)
		}
	}
	if len(query) > 0 {
		q := u.Query()
		for _, pair := range query {
			q.Add(pair[0], pair[1])
		}
		u.RawQuery = q.Encode()
	}
	return u, nil
}

func normalizeBase(baseURL string) (*url.URL, error) {
	s := strings.TrimSpace(baseURL)
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, invalidResource(err.Error())
	}
	return u, nil
}

func truncateBody(body string) string {
	const max = 512
	if utf8.RuneCountInString(body) <= max && len(body) <= max {
		return body
	}
	if len(body) <= max {
		return body
	}
	return body[:max] + "…"
}
