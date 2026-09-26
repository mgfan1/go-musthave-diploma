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
)

// ErrNotRegistered возвращается, когда система расчёта отвечает 204: заказ
// ей пока неизвестен. Это не отказ в расчёте, заказ нужно опросить позже.
var ErrNotRegistered = errors.New("заказ не зарегистрирован в системе расчёта")

// TooManyRequestsError возвращается, когда система расчёта отвечает 429.
type TooManyRequestsError struct {
	// RetryAfter хранит паузу из заголовка Retry-After. Если заголовка нет
	// или его не удалось разобрать, пауза равна одной минуте.
	RetryAfter time.Duration
}

// Error описывает ошибку вместе с запрошенной паузой.
func (e *TooManyRequestsError) Error() string {
	return fmt.Sprintf("система расчёта просит паузу %s", e.RetryAfter)
}

// Client запрашивает расчёт начислений у внешней системы по HTTP.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient создаёт клиент системы расчёта с базовым адресом baseURL вида
// http://host:port. На каждый запрос отводится пять секунд.
func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: requestTimeout}}
}

type orderResponse struct {
	Status  string       `json:"status"`
	Accrual *model.Money `json:"accrual"`
}

// Order запрашивает расчёт по заказу number и переводит ответ в статусы
// Гофермарта: REGISTERED и PROCESSING становятся PROCESSING, INVALID
// и PROCESSED остаются как есть. При ответе 204 возвращает ErrNotRegistered,
// при 429 возвращает *TooManyRequestsError. Прочие коды ответа, неизвестный
// статус и сбои сети возвращаются обычной ошибкой: такой заказ стоит
// опросить позже.
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
		return model.AccrualResult{}, &TooManyRequestsError{RetryAfter: retryAfter(resp.Header.Get("Retry-After"), time.Now())}
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

func retryAfter(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)

	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(header); err == nil {
		return max(at.Sub(now), 0)
	}

	return defaultRetryAfter
}
