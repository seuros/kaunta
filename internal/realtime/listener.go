package realtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/logging"
	"go.uber.org/zap"
)

const ChannelName = "kaunta_realtime_events"

type EventPayload struct {
	Type      string    `json:"type"`
	WebsiteID string    `json:"website_id"`
	SessionID string    `json:"session_id"`
	VisitID   string    `json:"visit_id"`
	Path      string    `json:"path,omitempty"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func NotifyEvent(ctx context.Context, payload EventPayload) {
	data, err := json.Marshal(payload)
	if err != nil {
		logging.L().Warn("failed to marshal realtime payload", zap.Error(err))
		return
	}

	if _, err := database.DB.ExecContext(ctx, "SELECT pg_notify($1, $2)", ChannelName, string(data)); err != nil {
		logging.L().Warn("failed to send realtime notification", zap.Error(err))
	}
}

func StartListener(ctx context.Context, databaseURL string, hub *Hub) error {
	listener := pq.NewListener(databaseURL, 5*time.Second, time.Minute, func(event pq.ListenerEventType, err error) {
		if err != nil {
			logging.L().Warn("realtime listener event", zap.Int("event", int(event)), zap.Error(err))
		}
	})

	if err := listener.Listen(ChannelName); err != nil {
		return err
	}

	go func() {
		defer func() {
			_ = listener.Close()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case n := <-listener.Notify:
				if n == nil {
					continue
				}
				hub.Broadcast([]byte(n.Extra))
			case <-time.After(time.Minute):
				if err := listener.Ping(); err != nil {
					logging.L().Warn("realtime listener ping failed", zap.Error(err))
				}
			}
		}
	}()

	return nil
}

func NewEventPayload(eventType string, websiteID, sessionID, visitID uuid.UUID, path, title string, createdAt time.Time) EventPayload {
	return EventPayload{
		Type:      eventType,
		WebsiteID: websiteID.String(),
		SessionID: sessionID.String(),
		VisitID:   visitID.String(),
		Path:      path,
		Title:     title,
		CreatedAt: createdAt,
	}
}
