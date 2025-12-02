package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/models"
)

type goalResponse struct {
	ID        string `json:"id"`
	WebsiteID string `json:"website_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`  // "page_view" or "custom_event"
	Value     string `json:"value"` // the URL or event name
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// HandleGoalList → GET /api/goals/:website_id
func HandleGoalList(c fiber.Ctx) error {
	websiteID := c.Params("website_id")
	if _, err := uuid.Parse(websiteID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid website_id"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := database.DB.QueryContext(ctx,
		`SELECT id, website_id, name, target_url, target_event, created_at, updated_at
		 FROM goals
		 WHERE website_id = $1
		 ORDER BY created_at DESC`, websiteID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database error"})
	}
	defer func() { _ = rows.Close() }()

	var goals []models.Goal
	for rows.Next() {
		var g models.Goal
		if err := rows.Scan(&g.ID, &g.WebsiteID, &g.Name, &g.TargetURL, &g.TargetEvent, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "scan error"})
		}
		goals = append(goals, g)
	}

	if err := rows.Err(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "rows error"})
	}

	var response []goalResponse
	for _, g := range goals {
		typ := "page_view"
		val := ""
		if g.TargetEvent != nil && *g.TargetEvent != "" {
			typ = "custom_event"
			val = *g.TargetEvent
		} else if g.TargetURL != nil {
			val = *g.TargetURL
		}

		response = append(response, goalResponse{
			ID:        g.ID,
			WebsiteID: g.WebsiteID,
			Name:      g.Name,
			Type:      typ,
			Value:     val,
			CreatedAt: g.CreatedAt.Format(time.RFC3339),
			UpdatedAt: g.UpdatedAt.Format(time.RFC3339),
		})
	}
	return c.JSON(response)
}

// HandleGoalCreate   → POST /api/goals
func HandleGoalCreate(c fiber.Ctx) error {
	var req struct {
		WebsiteID string `json:"website_id" validate:"required,uuid4"`
		Name      string `json:"name" validate:"required"`
		Type      string `json:"type" validate:"required"`
		Value     string `json:"value" validate:"required"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	// Convert frontend format to DB format
	var targetURL, targetEvent *string
	if req.Type == "page_view" {
		targetURL = &req.Value
	} else if req.Type == "custom_event" {
		targetEvent = &req.Value
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "invalid goal type"})
	}

	id := uuid.New().String()
	now := time.Now()
	_, err := database.DB.Exec(
		`INSERT INTO goals (id, website_id, name, target_url, target_event, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, req.WebsiteID, req.Name, targetURL, targetEvent, now, now)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create goal"})
	}

	return c.Status(201).JSON(goalResponse{
		ID:        id,
		WebsiteID: req.WebsiteID,
		Name:      req.Name,
		Type:      req.Type,
		Value:     req.Value,
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
	})
}

// HandleGoalUpdate   → PUT  /api/goals/:id
func HandleGoalUpdate(c fiber.Ctx) error {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid goal id"})
	}

	var req struct {
		Name  string `json:"name" validate:"required"`
		Type  string `json:"type" validate:"required"`
		Value string `json:"value" validate:"required"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	// Convert frontend format to DB format
	var targetURL, targetEvent *string
	if req.Type == "page_view" {
		targetURL = &req.Value
	} else if req.Type == "custom_event" {
		targetEvent = &req.Value
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "invalid goal type"})
	}

	now := time.Now()
	_, err := database.DB.Exec(
		`UPDATE goals SET name = $1, target_url = $2, target_event = $3, updated_at = $4
		 WHERE id = $5`,
		req.Name, targetURL, targetEvent, now, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}

	return c.JSON(goalResponse{
		ID:        id,
		WebsiteID: "", // We don't have this in update context
		Name:      req.Name,
		Type:      req.Type,
		Value:     req.Value,
		CreatedAt: "", // We don't have this in update context
		UpdatedAt: now.Format(time.RFC3339),
	})
}

// HandleGoalDelete   → DELETE /api/goals/:id
func HandleGoalDelete(c fiber.Ctx) error {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid goal id"})
	}

	_, err := database.DB.Exec(`DELETE FROM goals WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "delete failed"})
	}
	return c.SendStatus(204)
}
