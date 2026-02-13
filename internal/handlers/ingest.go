package handlers

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/geoip"
	"github.com/seuros/kaunta/internal/httpx"
	"github.com/seuros/kaunta/internal/logging"
	"github.com/seuros/kaunta/internal/middleware"
	"github.com/seuros/kaunta/internal/models"
	"go.uber.org/zap"
)

// Global validator instance
var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())

	// Register custom validation: url required for page_view events
	validate.RegisterStructValidation(func(sl validator.StructLevel) {
		payload := sl.Current().Interface().(IngestPayload)
		if payload.Event == "page_view" && payload.URL == "" {
			sl.ReportError(payload.URL, "url", "URL", "required_for_pageview", "")
		}
	}, IngestPayload{})
}

// IngestPayload represents a single event for server-side ingestion
type IngestPayload struct {
	Event       string                 `json:"event" validate:"required,max=50"`
	VisitorID   string                 `json:"visitor_id" validate:"required,max=500"`
	URL         string                 `json:"url" validate:"omitempty,max=2000"`
	Hostname    string                 `json:"hostname" validate:"omitempty,max=100"`
	Referrer    string                 `json:"referrer" validate:"omitempty,max=2000"`
	Title       string                 `json:"title" validate:"omitempty,max=500"`
	UserID      *string                `json:"user_id" validate:"omitempty,max=500"`
	SessionID   *string                `json:"session_id" validate:"omitempty,max=100"`
	EventID     *string                `json:"event_id" validate:"omitempty,uuid4"`
	Timestamp   *int64                 `json:"timestamp"`
	Properties  map[string]interface{} `json:"properties"`
	Context     *IngestContext         `json:"context"`
	UTMSource   *string                `json:"utm_source" validate:"omitempty,max=100"`
	UTMMedium   *string                `json:"utm_medium" validate:"omitempty,max=100"`
	UTMCampaign *string                `json:"utm_campaign" validate:"omitempty,max=100"`
	UTMTerm     *string                `json:"utm_term" validate:"omitempty,max=100"`
	UTMContent  *string                `json:"utm_content" validate:"omitempty,max=100"`
}

// IngestContext contains optional context metadata
type IngestContext struct {
	Locale string `json:"locale" validate:"omitempty,max=20"`
	Screen string `json:"screen" validate:"omitempty,max=20"`
}

// BatchIngestRequest represents a batch of events
type BatchIngestRequest struct {
	Events []IngestPayload `json:"events" validate:"required,min=1,max=100,dive"`
}

// BatchIngestResponse contains results of batch processing
type BatchIngestResponse struct {
	Accepted int          `json:"accepted"`
	Failed   int          `json:"failed"`
	Errors   []BatchError `json:"errors,omitempty"`
}

// BatchError describes a single failed event in a batch
type BatchError struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

// HandleIngest processes single event ingestion from server-side clients
// POST /api/ingest
func HandleIngest(w http.ResponseWriter, r *http.Request) {
	apiKey := middleware.GetAPIKey(r)
	if apiKey == nil {
		httpx.Error(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var payload IngestPayload
	if err := httpx.ReadJSON(r, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := validateIngestPayload(&payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if payload.EventID != nil {
		eventUUID, err := uuid.Parse(*payload.EventID)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "Invalid event_id format")
			return
		}

		exists, err := models.CheckEventIDExists(eventUUID, apiKey.WebsiteID)
		if err != nil {
			logging.L().Warn("idempotency check failed", zap.Error(err))
		} else if exists {
			httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
				"status":     "accepted",
				"idempotent": true,
			})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := processIngestEvent(ctx, r, apiKey, &payload)
	if err != nil {
		logging.L().Error("failed to process ingest event",
			zap.String("website_id", apiKey.WebsiteID.String()),
			zap.Error(err))
		httpx.Error(w, http.StatusInternalServerError, "Failed to process event")
		return
	}

	httpx.WriteJSON(w, http.StatusAccepted, result)
}

// HandleIngestBatch processes batch event ingestion
// POST /api/ingest/batch
func HandleIngestBatch(w http.ResponseWriter, r *http.Request) {
	apiKey := middleware.GetAPIKey(r)
	if apiKey == nil {
		httpx.Error(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var request BatchIngestRequest
	if err := httpx.ReadJSON(r, &request); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if len(request.Events) == 0 {
		httpx.Error(w, http.StatusBadRequest, "Events array is required")
		return
	}

	if len(request.Events) > 100 {
		httpx.Error(w, http.StatusBadRequest, "Maximum 100 events per batch")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	response := BatchIngestResponse{
		Errors: []BatchError{},
	}

	for i, payload := range request.Events {
		select {
		case <-ctx.Done():
			httpx.WriteJSON(w, http.StatusAccepted, response)
			return
		default:
		}

		if err := validateIngestPayload(&payload); err != nil {
			response.Failed++
			response.Errors = append(response.Errors, BatchError{
				Index: i,
				Error: err.Error(),
			})
			continue
		}

		if payload.EventID != nil {
			eventUUID, err := uuid.Parse(*payload.EventID)
			if err == nil {
				exists, _ := models.CheckEventIDExists(eventUUID, apiKey.WebsiteID)
				if exists {
					response.Accepted++
					continue
				}
			}
		}

		if _, err := processIngestEvent(ctx, r, apiKey, &payload); err != nil {
			response.Failed++
			response.Errors = append(response.Errors, BatchError{
				Index: i,
				Error: "processing failed",
			})
			continue
		}

		response.Accepted++
	}

	httpx.WriteJSON(w, http.StatusAccepted, response)
}

// processIngestEvent handles the core event processing logic
func processIngestEvent(ctx context.Context, r *http.Request, apiKey *models.APIKey, payload *IngestPayload) (map[string]any, error) {
	websiteID := apiKey.WebsiteID

	// Get website proxy_mode for IP resolution
	var proxyMode string
	err := database.DB.QueryRowContext(ctx,
		"SELECT COALESCE(proxy_mode, 'none') FROM website WHERE website_id = $1",
		websiteID,
	).Scan(&proxyMode)
	if err != nil {
		return nil, fmt.Errorf("website not found: %w", err)
	}

	// Get client IP from headers (NEVER from payload for security)
	ip := clientIPFromRequest(r, proxyMode)
	userAgent := r.Header.Get("User-Agent")

	// Bot detection
	var isBot *bool
	err = database.DB.QueryRowContext(ctx, `
		SELECT update_ip_metadata($1::inet, $2, NULL)
	`, ip, userAgent).Scan(&isBot)

	if err != nil {
		logging.L().Warn("bot detection error", zap.String("ip", ip), zap.Error(err))
		isBotVal := false
		isBot = &isBotVal
	}

	if isBot != nil && *isBot {
		return map[string]any{"status": "accepted", "bot_detected": true}, nil
	}

	// Parse client info from User-Agent
	browser, os, device := parseUserAgent(userAgent)

	// GeoIP lookup
	countryStr, cityStr, regionStr := geoip.LookupIP(ip)
	country := &countryStr
	region := &regionStr
	city := &cityStr

	// Determine timestamp
	createdAt := time.Now()
	if payload.Timestamp != nil {
		t := time.Unix(*payload.Timestamp, 0)
		now := time.Now()
		// Validate timestamp within ±30 days
		if !t.Before(now.Add(-30*24*time.Hour)) && !t.After(now.Add(30*24*time.Hour)) {
			createdAt = t
		}
	}

	// Generate or use provided session ID
	sessionID := resolveSessionID(payload, websiteID, ip, userAgent, createdAt)

	// Parse URL path
	var urlPath *string
	var urlQuery *string
	var hostname *string
	if payload.URL != "" {
		if u, err := url.Parse(payload.URL); err == nil {
			path := u.Path
			urlPath = &path
			if u.RawQuery != "" {
				q := u.RawQuery
				urlQuery = &q
			}
			if payload.Hostname != "" {
				hostname = &payload.Hostname
			} else {
				h := u.Hostname()
				hostname = &h
			}
		}
	}

	// Screen from context
	var screen *string
	if payload.Context != nil && payload.Context.Screen != "" {
		screen = &payload.Context.Screen
	}

	// Language from context
	var language *string
	if payload.Context != nil && payload.Context.Locale != "" {
		language = &payload.Context.Locale
	}

	// Upsert session
	err = upsertSessionForIngest(ctx, sessionID, websiteID, browser, os, device,
		screen, language, country, region, city, payload.UserID, urlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Generate visit ID (hourly)
	visitSalt := hashDate(createdAt, "hour")
	visitID := generateUUID(sessionID.String(), visitSalt)

	// Save event
	eventID, err := saveIngestEvent(ctx, websiteID, sessionID, visitID, createdAt, payload,
		browser, os, device, country, region, city, hostname, urlPath, urlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to save event: %w", err)
	}

	// Record idempotency key if provided
	if payload.EventID != nil {
		eventUUID, err := uuid.Parse(*payload.EventID)
		if err == nil {
			_ = models.InsertEventID(eventUUID, websiteID)
		}
	}

	// Check goals
	eventType := 1 // pageview
	if payload.Event != "page_view" {
		eventType = 2 // custom event
	}

	eventName := &payload.Event
	if payload.Event == "page_view" {
		eventName = nil
	}

	goalID := checkAndRecordGoalCompletion(ctx, websiteID, sessionID, eventID, eventType, urlPath, eventName)
	if goalID != nil {
		_, _ = database.DB.ExecContext(ctx,
			`UPDATE website_event SET goal_id = $1 WHERE event_id = $2 AND created_at = $3`,
			goalID, eventID, createdAt,
		)
	}

	return map[string]any{
		"status":     "accepted",
		"session_id": sessionID.String(),
		"visit_id":   visitID.String(),
	}, nil
}

// validateIngestPayload validates the ingest payload using go-playground/validator
func validateIngestPayload(p *IngestPayload) error {
	// Struct validation
	if err := validate.Struct(p); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			return formatIngestValidationError(validationErrors[0])
		}
		return err
	}

	// Timestamp validation (custom)
	if p.Timestamp != nil {
		t := time.Unix(*p.Timestamp, 0)
		now := time.Now()
		if t.Before(now.Add(-30*24*time.Hour)) || t.After(now.Add(30*24*time.Hour)) {
			return errors.New("timestamp must be within 30 days of now")
		}
	}

	// Properties validation
	if p.Properties != nil {
		if err := validateIngestProperties(p.Properties); err != nil {
			return err
		}
	}

	return nil
}

// formatIngestValidationError converts validator errors to user-friendly messages
func formatIngestValidationError(fe validator.FieldError) error {
	switch fe.Tag() {
	case "required":
		return fmt.Errorf("%s is required", strings.ToLower(fe.Field()))
	case "max":
		return fmt.Errorf("%s exceeds maximum length of %s", strings.ToLower(fe.Field()), fe.Param())
	case "uuid4":
		return fmt.Errorf("%s must be a valid UUID v4", strings.ToLower(fe.Field()))
	case "required_for_pageview":
		return errors.New("url is required for page_view events")
	default:
		return fmt.Errorf("%s is invalid", strings.ToLower(fe.Field()))
	}
}

// validateIngestProperties checks property constraints
func validateIngestProperties(props map[string]interface{}) error {
	// Max 100KB
	jsonBytes, err := json.Marshal(props)
	if err != nil {
		return errors.New("invalid properties format")
	}
	if len(jsonBytes) > 100*1024 {
		return errors.New("properties exceed 100KB limit")
	}

	// Max 100 keys
	if len(props) > 100 {
		return errors.New("properties exceed 100 keys limit")
	}

	// No reserved prefixes
	for key := range props {
		if strings.HasPrefix(key, "$") || strings.HasPrefix(key, "_") {
			return fmt.Errorf("reserved property key: %s", key)
		}
	}

	// Max 5 depth
	if getJSONDepth(props, 0) > 5 {
		return errors.New("properties exceed max depth of 5")
	}

	return nil
}

// getJSONDepth calculates the depth of a JSON object
func getJSONDepth(data interface{}, currentDepth int) int {
	maxDepth := currentDepth

	switch v := data.(type) {
	case map[string]interface{}:
		for _, value := range v {
			depth := getJSONDepth(value, currentDepth+1)
			if depth > maxDepth {
				maxDepth = depth
			}
		}
	case []interface{}:
		for _, item := range v {
			depth := getJSONDepth(item, currentDepth+1)
			if depth > maxDepth {
				maxDepth = depth
			}
		}
	}

	return maxDepth
}

// resolveSessionID generates or resolves a session ID
func resolveSessionID(payload *IngestPayload, websiteID uuid.UUID, ip, userAgent string, createdAt time.Time) uuid.UUID {
	// If explicit session_id provided, use it
	if payload.SessionID != nil && *payload.SessionID != "" {
		// Try parsing as UUID first
		if id, err := uuid.Parse(*payload.SessionID); err == nil {
			return id
		}
		// Otherwise generate deterministic UUID from the string
		return generateDeterministicUUID(websiteID.String(), *payload.SessionID)
	}

	// Generate session from visitor_id + hour (for server-side, use hour granularity)
	hourSalt := hashDate(createdAt, "hour")
	return generateDeterministicUUID(websiteID.String(), payload.VisitorID, hourSalt)
}

// generateDeterministicUUID creates a UUID from arbitrary strings
func generateDeterministicUUID(parts ...string) uuid.UUID {
	combined := strings.Join(parts, "|")
	hash := md5.Sum([]byte(combined))
	id, _ := uuid.FromBytes(hash[:])
	return id
}

// upsertSessionForIngest creates or updates a session for ingested events
func upsertSessionForIngest(ctx context.Context, sessionID, websiteID uuid.UUID,
	browser, os, device, screen, language, country, region, city *string,
	distinctID *string, urlPath *string) error {

	query := `
		INSERT INTO session (
			session_id, website_id, browser, os, device, screen, language,
			country, region, city, created_at, distinct_id, entry_page, exit_page
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), $11, $12, $12)
		ON CONFLICT (session_id) DO UPDATE SET exit_page = EXCLUDED.entry_page
	`
	_, err := database.DB.ExecContext(ctx, query, sessionID, websiteID, browser, os, device,
		screen, language, country, region, city, distinctID, urlPath)
	return err
}

// saveIngestEvent saves an event from the ingest API
func saveIngestEvent(ctx context.Context, websiteID, sessionID, visitID uuid.UUID, createdAt time.Time,
	payload *IngestPayload, browser, os, device, country, region, city *string,
	hostname, urlPath, urlQuery *string) (uuid.UUID, error) {

	eventID := uuid.New()
	eventType := 1 // pageview
	var eventName *string

	if payload.Event != "page_view" {
		eventType = 2 // custom event
		eventName = &payload.Event
	}

	// Parse referrer
	var referrerPath, referrerQuery, referrerDomain *string
	if payload.Referrer != "" {
		if u, err := url.Parse(payload.Referrer); err == nil {
			path := u.Path
			referrerPath = &path
			if u.RawQuery != "" {
				q := u.RawQuery
				referrerQuery = &q
			}
			domain := strings.TrimPrefix(u.Hostname(), "www.")
			if domain != "localhost" && domain != "" {
				referrerDomain = &domain
			}
		}
	}

	// Convert properties to JSON
	var propsJSON interface{}
	if len(payload.Properties) > 0 {
		jsonBytes, _ := json.Marshal(payload.Properties)
		propsJSON = jsonBytes
	}

	// Title
	var title *string
	if payload.Title != "" {
		title = &payload.Title
	}

	query := `
		INSERT INTO website_event (
			event_id, website_id, session_id, visit_id, created_at,
			page_title, hostname, url_path, url_query,
			referrer_path, referrer_query, referrer_domain,
			event_name, event_type, props,
			utm_source, utm_medium, utm_campaign, utm_term, utm_content
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12,
			$13, $14, $15,
			$16, $17, $18, $19, $20
		)
	`

	_, err := database.DB.ExecContext(ctx, query,
		eventID, websiteID, sessionID, visitID, createdAt,
		title, hostname, urlPath, urlQuery,
		referrerPath, referrerQuery, referrerDomain,
		eventName, eventType, propsJSON,
		payload.UTMSource, payload.UTMMedium, payload.UTMCampaign, payload.UTMTerm, payload.UTMContent,
	)

	if err != nil {
		logging.L().Error("failed to insert ingest event",
			zap.String("event_id", eventID.String()),
			zap.Error(err))
		return uuid.Nil, err
	}

	return eventID, nil
}
