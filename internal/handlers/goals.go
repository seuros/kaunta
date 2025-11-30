package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/models"
)

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

	return c.JSON(goals)
}

// HandleGoalCreate   → POST /api/goals
func HandleGoalCreate(c fiber.Ctx) error {
	var req struct {
		WebsiteID   string  `json:"website_id" validate:"required,uuid4"`
		Name        string  `json:"name" validate:"required"`
		TargetURL   *string `json:"target_url"`
		TargetEvent *string `json:"target_event"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	id := uuid.New().String()
	_, err := database.DB.Exec(
		`INSERT INTO goals (id, website_id, name, target_url, target_event, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`,
		id, req.WebsiteID, req.Name, req.TargetURL, req.TargetEvent)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create goal"})
	}

	return c.Status(201).JSON(models.Goal{
		ID:          id,
		WebsiteID:   req.WebsiteID,
		Name:        req.Name,
		TargetURL:   req.TargetURL,
		TargetEvent: req.TargetEvent,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})
}

// HandleGoalUpdate   → PUT  /api/goals/:id
func HandleGoalUpdate(c fiber.Ctx) error {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid goal id"})
	}

	var req struct {
		Name        *string `json:"name"`
		TargetURL   *string `json:"target_url"`
		TargetEvent *string `json:"target_event"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	_, err := database.DB.Exec(
		`UPDATE goals SET name = COALESCE($1, name), target_url = $2, target_event = $3, updated_at = NOW()
		 WHERE id = $4`,
		req.Name, req.TargetURL, req.TargetEvent, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}
	return c.SendStatus(200)
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
