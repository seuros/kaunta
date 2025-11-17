package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSetupForm(t *testing.T) {
	tests := []struct {
		name    string
		form    SetupForm
		wantErr string
	}{
		{
			name: "valid form",
			form: SetupForm{
				DBHost:               "localhost",
				DBPort:               "5432",
				DBName:               "kaunta",
				DBUser:               "postgres",
				DBPassword:           "password",
				AdminUsername:        "admin",
				AdminName:            "Admin User",
				AdminPassword:        "password123",
				AdminPasswordConfirm: "password123",
			},
			wantErr: "",
		},
		{
			name: "missing database host",
			form: SetupForm{
				DBPort:               "5432",
				DBName:               "kaunta",
				DBUser:               "postgres",
				AdminUsername:        "admin",
				AdminName:            "Admin User",
				AdminPassword:        "password123",
				AdminPasswordConfirm: "password123",
			},
			wantErr: "database host is required",
		},
		{
			name: "missing database name",
			form: SetupForm{
				DBHost:               "localhost",
				DBPort:               "5432",
				DBUser:               "postgres",
				AdminUsername:        "admin",
				AdminName:            "Admin User",
				AdminPassword:        "password123",
				AdminPasswordConfirm: "password123",
			},
			wantErr: "database name is required",
		},
		{
			name: "short username",
			form: SetupForm{
				DBHost:               "localhost",
				DBPort:               "5432",
				DBName:               "kaunta",
				DBUser:               "postgres",
				AdminUsername:        "ab",
				AdminName:            "Admin User",
				AdminPassword:        "password123",
				AdminPasswordConfirm: "password123",
			},
			wantErr: "username must be between 3 and 30 characters",
		},
		{
			name: "invalid username characters",
			form: SetupForm{
				DBHost:               "localhost",
				DBPort:               "5432",
				DBName:               "kaunta",
				DBUser:               "postgres",
				AdminUsername:        "admin@user",
				AdminName:            "Admin User",
				AdminPassword:        "password123",
				AdminPasswordConfirm: "password123",
			},
			wantErr: "username can only contain letters, numbers, and underscores",
		},
		{
			name: "short password",
			form: SetupForm{
				DBHost:               "localhost",
				DBPort:               "5432",
				DBName:               "kaunta",
				DBUser:               "postgres",
				AdminUsername:        "admin",
				AdminName:            "Admin User",
				AdminPassword:        "pass",
				AdminPasswordConfirm: "pass",
			},
			wantErr: "password must be at least 8 characters",
		},
		{
			name: "password mismatch",
			form: SetupForm{
				DBHost:               "localhost",
				DBPort:               "5432",
				DBName:               "kaunta",
				DBUser:               "postgres",
				AdminUsername:        "admin",
				AdminName:            "Admin User",
				AdminPassword:        "password123",
				AdminPasswordConfirm: "different",
			},
			wantErr: "passwords do not match",
		},
		{
			name: "defaults applied",
			form: SetupForm{
				DBHost:               "localhost",
				DBPort:               "", // Should default to 5432
				DBName:               "kaunta",
				DBUser:               "postgres",
				AdminUsername:        "admin",
				AdminName:            "Admin User",
				AdminPassword:        "password123",
				AdminPasswordConfirm: "password123",
				ServerPort:           "", // Should default to 3000
				DataDir:              "", // Should default to ./data
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSetupForm(&tt.form)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				// Check defaults were applied (only for defaults_applied test case)
				if tt.name == "defaults applied" {
					assert.Equal(t, "3000", tt.form.ServerPort)
					assert.Equal(t, "./data", tt.form.DataDir)
				}
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestBuildDatabaseURL(t *testing.T) {
	tests := []struct {
		name     string
		form     SetupForm
		expected string
	}{
		{
			name: "with password",
			form: SetupForm{
				DBHost:     "localhost",
				DBPort:     "5432",
				DBName:     "kaunta",
				DBUser:     "postgres",
				DBPassword: "secret",
				DBSSLMode:  "disable",
			},
			expected: "postgres://postgres:secret@localhost:5432/kaunta?sslmode=disable",
		},
		{
			name: "without password",
			form: SetupForm{
				DBHost:    "localhost",
				DBPort:    "5432",
				DBName:    "kaunta",
				DBUser:    "postgres",
				DBSSLMode: "require",
			},
			expected: "postgres://postgres@localhost:5432/kaunta?sslmode=require",
		},
		{
			name: "default port and ssl",
			form: SetupForm{
				DBHost: "localhost",
				DBPort: "",
				DBName: "kaunta",
				DBUser: "postgres",
			},
			expected: "postgres://postgres@localhost:5432/kaunta?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildDatabaseURL(&tt.form)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTestDatabase(t *testing.T) {
	app := fiber.New()
	app.Post("/test", TestDatabase())

	tests := []struct {
		name         string
		form         SetupForm
		expectedCode int
		checkError   bool
	}{
		{
			name: "missing required fields",
			form: SetupForm{
				DBHost: "",
			},
			expectedCode: 400,
			checkError:   true,
		},
		{
			name: "valid form (will fail connection)",
			form: SetupForm{
				DBHost: "localhost",
				DBPort: "5432",
				DBName: "test",
				DBUser: "test",
			},
			expectedCode: 400, // Connection will fail in test
			checkError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.form)
			req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCode, resp.StatusCode)

			var result map[string]interface{}
			_ = json.NewDecoder(resp.Body).Decode(&result)

			if tt.checkError {
				assert.Contains(t, result, "error")
			}
		})
	}
}
