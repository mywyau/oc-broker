package event

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	ClaimSubmitted = "claim.submitted"
	ClaimProcessed = "claim.processed"
)

type Envelope struct {
	EventID       uuid.UUID       `json:"eventId"`
	EventType     string          `json:"eventType"`
	EventVersion  int             `json:"eventVersion"`
	OccurredAt    time.Time       `json:"occurredAt"`
	CorrelationID uuid.UUID       `json:"correlationId"`
	Payload       json.RawMessage `json:"payload"`
}

type ClaimSubmittedPayload struct {
	ClaimID uuid.UUID `json:"claimId"`
}

type ClaimProcessedPayload struct {
	ClaimID uuid.UUID `json:"claimId"`
	Status  string    `json:"status"`
	Reason  string    `json:"reason,omitempty"`
}
