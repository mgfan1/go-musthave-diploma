package accrual

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

const (
	requestTimeout    = 5 * time.Second
	defaultRetryAfter = time.Minute
	maxRetryAfter     = 10 * time.Minute
)

// ErrNotRegistered возвращается, когда система расчёта отвечает 204: заказ
// ей пока неизвестен. Это не отказ в расчёте, заказ нужно опросить позже.
var ErrNotRegistered = errors.New("заказ не зарегистрирован в системе расчёта")

// TooManyRequestsError возвращается, когда система расчёта отвечает 429.
type TooManyRequestsError struct {
	// RetryAfter - пауза из заголовка Retry-After в секундах, не длиннее
	// десяти минут. Без заголовка или с неразборчивым значением берётся минута.
	RetryAfter time.Duration
}

// Error сообщает, какую паузу просит система расчёта.
func (e *TooManyRequestsError) Error() string {
	return fmt.Sprintf("система расчёта просит паузу %s", e.RetryAfter)
}

// Client запрашивает расчёт начислений у внешней системы по HTTP.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient создаёт клиент системы расчёта с базовым адресом baseURL вида
// http://host:port.
func NewClient(baseURL string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxConnsPerHost = workers
	transport.MaxIdleConnsPerHost = workers

	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: requestTimeout, Transport: transport},
	}
}

type orderResponse struct {
	Status  string       `json:"status"`
	Accrual *model.Money `json:"accrual"`
}

// Order запрашивает расчёт по заказу number. REGISTERED приводится
// к PROCESSING. На ответ 204 возвращает ErrNotRegistered, на 429
// *TooManyRequestsError, на прочие сбои обычную ошибку.
func (c *Client) Order(ctx context.Context, number string) (model.AccrualResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/orders/"+url.PathEscape(number), nil)
	if err != nil {
		return model.AccrualResult{}, fmt.Errorf("не собрал запрос к системе расчёта: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return model.AccrualResult{}, fmt.Errorf("не получил ответ системы расчёта: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		return decodeOrder(resp.Body)
	case http.StatusNoContent:
		return model.AccrualResult{}, ErrNotRegistered
	case http.StatusTooManyRequests:
		return model.AccrualResult{}, &TooManyRequestsError{RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	default:
		return model.AccrualResult{}, fmt.Errorf("система расчёта ответила %s", resp.Status)
	}
}

func decodeOrder(r io.Reader) (model.AccrualResult, error) {
	var body orderResponse
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return model.AccrualResult{}, fmt.Errorf("не разобрал ответ системы расчёта: %w", err)
	}

	status, err := orderStatus(body.Status)
	if err != nil {
		return model.AccrualResult{}, err
	}

	return model.AccrualResult{Status: status, Amount: body.Accrual}, nil
}

func orderStatus(s string) (model.OrderStatus, error) {
	switch s {
	case "REGISTERED", "PROCESSING":
		return model.StatusProcessing, nil
	case "INVALID":
		return model.StatusInvalid, nil
	case "PROCESSED":
		return model.StatusProcessed, nil
	default:
		return "", fmt.Errorf("неизвестный статус расчёта %q", s)
	}
}

func retryAfter(header string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil || seconds < 0 {
		return defaultRetryAfter
	}
	return time.Duration(min(seconds, int(maxRetryAfter/time.Second))) * time.Second
}
