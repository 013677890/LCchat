//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"testing"
	"time"
)

// APIResponse 是 Gateway 所有 JSON 接口共用的外层响应。
// Data 使用 RawMessage 保留原始 JSON，调用方可以按当前接口需要解码为结构体或 map。
type APIResponse struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
	TraceID   string          `json:"trace_id"`
	Timestamp int64           `json:"timestamp"`
}

// HTTPResponse 同时保存 HTTP 状态和业务响应，便于验证“HTTP 200 但业务 code 非 0”
// 这种项目约定，也能保留非 JSON 错误页用于排障。
type HTTPResponse struct {
	Status int
	Body   APIResponse
	Raw    []byte
}

// HTTPClient 是面向真实 Gateway 的轻量测试客户端，不依赖第三方 HTTP 测试框架。
type HTTPClient struct {
	BaseURL string
	Client  *http.Client
}

func NewHTTPClient(cfg Config) *HTTPClient {
	return &HTTPClient{
		BaseURL: strings.TrimRight(cfg.GatewayBase, "/"),
		Client:  &http.Client{Timeout: cfg.RequestTimeout},
	}
}

// DoJSON 发送 JSON 请求。body 可以是任意可 JSON 序列化对象，也可以传 json.RawMessage
// 以测试非法 JSON；token/deviceID 为空时不会添加对应请求头。
func (c *HTTPClient) DoJSON(ctx context.Context, method, path, token, deviceID string, body any) (HTTPResponse, error) {
	var reader io.Reader
	if body != nil {
		var payload []byte
		var err error
		switch value := body.(type) {
		case json.RawMessage:
			payload = value
		case []byte:
			payload = value
		default:
			payload, err = json.Marshal(body)
			if err != nil {
				return HTTPResponse{}, fmt.Errorf("序列化请求体失败: %w", err)
			}
		}
		reader = bytes.NewReader(payload)
	}
	return c.do(ctx, method, path, token, deviceID, reader, "application/json")
}

// DoMultipart 用于头像等文件上传接口。
// files 的 key 是表单字段名，value 是文件名、MIME、内容。
func (c *HTTPClient) DoMultipart(ctx context.Context, method, path, token, deviceID string, files map[string]UploadFile) (HTTPResponse, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for field, file := range files {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, file.Name))
		header.Set("Content-Type", file.MIME)
		part, err := writer.CreatePart(header)
		if err != nil {
			return HTTPResponse{}, fmt.Errorf("创建 multipart 文件字段失败: %w", err)
		}
		if _, err := part.Write(file.Content); err != nil {
			return HTTPResponse{}, fmt.Errorf("写入 multipart 文件失败: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return HTTPResponse{}, fmt.Errorf("关闭 multipart writer 失败: %w", err)
	}
	return c.do(ctx, method, path, token, deviceID, bytes.NewReader(buffer.Bytes()), writer.FormDataContentType())
}

// UploadFile 描述一个 multipart 文件字段。
type UploadFile struct {
	Name    string
	MIME    string
	Content []byte
}

func (c *HTTPClient) do(ctx context.Context, method, path, token, deviceID string, body io.Reader, contentType string) (HTTPResponse, error) {
	endpoint := path
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		endpoint = path
	} else {
		endpoint = c.BaseURL + "/" + strings.TrimLeft(path, "/")
	}
	var payload []byte
	if body != nil {
		var err error
		payload, err = io.ReadAll(body)
		if err != nil {
			return HTTPResponse{}, fmt.Errorf("读取请求体失败: %w", err)
		}
	}
	// Gateway 的用户级限流是正常运行时行为。测试客户端只对 429/code=10005
	// 做有限指数退避，避免把限流误报成业务功能失败；其他错误不重试。
	maxRetries := parseIntEnv("LCCHAT_RATE_LIMIT_RETRY_COUNT", 6)
	baseDelay := envDuration("LCCHAT_RATE_LIMIT_RETRY_BASE_DELAY", 250*time.Millisecond)
	for attempt := 0; ; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
		if err != nil {
			return HTTPResponse{}, fmt.Errorf("创建 HTTP 请求失败: %w", err)
		}
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		if deviceID != "" {
			request.Header.Set("X-Device-ID", deviceID)
		}
		response, err := c.Client.Do(request)
		if err != nil {
			return HTTPResponse{}, err
		}
		raw, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			return HTTPResponse{}, fmt.Errorf("读取 HTTP 响应失败: %w", readErr)
		}
		result := HTTPResponse{Status: response.StatusCode, Raw: raw}
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &result.Body); err != nil {
				return result, fmt.Errorf("响应不是合法 JSON: HTTP %d body=%q: %w", response.StatusCode, string(raw), err)
			}
		}
		if (result.Status == http.StatusTooManyRequests || result.Body.Code == 10005) && attempt < maxRetries {
			delay := baseDelay * time.Duration(1<<attempt)
			if delay > 3*time.Second {
				delay = 3 * time.Second
			}
			select {
			case <-ctx.Done():
				return HTTPResponse{}, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		return result, nil
	}
}

func (r HTTPResponse) DecodeData(target any) error {
	if len(r.Body.Data) == 0 || string(r.Body.Data) == "null" {
		return nil
	}
	return json.Unmarshal(r.Body.Data, target)
}

func (r HTTPResponse) Summary() string {
	return fmt.Sprintf("http=%d code=%d message=%s trace=%s", r.Status, r.Body.Code, r.Body.Message, r.Body.TraceID)
}

func requireHTTPSuccess(t *testing.T, name string, response HTTPResponse) {
	t.Helper()
	if response.Status != http.StatusOK || response.Body.Code != 0 {
		t.Fatalf("%s 预期成功，实际 %s body=%s", name, response.Summary(), string(response.Raw))
	}
}

func requireHTTPError(t *testing.T, name string, response HTTPResponse) {
	t.Helper()
	if response.Status == http.StatusOK && response.Body.Code == 0 {
		t.Fatalf("%s 预期失败但接口成功: body=%s", name, string(response.Raw))
	}
}

func requireJSONData(t *testing.T, name string, response HTTPResponse, target any) {
	t.Helper()
	requireHTTPSuccess(t, name, response)
	if err := response.DecodeData(target); err != nil {
		t.Fatalf("%s 解码 data 失败: %v; body=%s", name, err, string(response.Raw))
	}
}

func queryPath(path string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		if strings.Contains(path, "?") {
			return path + "&" + encoded
		}
		return path + "?" + encoded
	}
	return path
}
