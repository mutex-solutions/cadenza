package expopush

import (
	"context"
	"encoding/json"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Receipt is Expo's final word on one ticket, available some minutes after
// the send. Status is "ok" or "error"; Error is details.error, for example
// "DeviceNotRegistered".
type Receipt struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Gone reports whether the receipt says the device token is no longer valid.
func (r Receipt) Gone() bool { return r.Error == "DeviceNotRegistered" }

// CheckReceipts asks Expo for the receipts of these ticket ids. Ids with no
// receipt yet are missing from the map.
func (s *Sender) CheckReceipts(ctx context.Context, ids []string) (map[string]Receipt, error) {
	if len(ids) == 0 {
		return map[string]Receipt{}, nil
	}
	body, err := json.Marshal(struct {
		IDs []string `json:"ids"`
	}{ids})
	if err != nil {
		return nil, kernel.Internal(err, "encode expo receipt ids")
	}
	var resp struct {
		Data map[string]struct {
			Status  string  `json:"status"`
			Message string  `json:"message"`
			Details details `json:"details"`
		} `json:"data"`
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := s.post(ctx, "/--/api/v2/push/getReceipts", body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		return nil, kernel.Unavailable("expo rejected the receipt request: %s %s", resp.Errors[0].Code, resp.Errors[0].Message)
	}
	out := make(map[string]Receipt, len(resp.Data))
	for id, r := range resp.Data {
		out[id] = Receipt{Status: r.Status, Message: r.Message, Error: r.Details.Error}
	}
	return out, nil
}
