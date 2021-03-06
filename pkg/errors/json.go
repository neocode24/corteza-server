package errors

import "encoding/json"

func (e Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Message string   `json:"message"`
		Meta    meta     `json:"meta,omitempty"`
		Stack   []*frame `json:"stack,omitempty"`
		Wrap    error    `json:"wrap,omitempty"`
	}{
		Message: e.Error(),
		Meta:    e.meta,
		Stack:   e.stack,
		Wrap:    e.wrap,
	})
}
