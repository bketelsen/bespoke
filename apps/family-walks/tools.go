// LLM tools (ADR-0021): log the walk and check the streak, scoped to the
// calling user.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/web"
)

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func registerTools(mux *http.ServeMux, sqldb *sql.DB) {
	web.Tool(mux, web.ToolDef{
		Name:        "log_walk",
		Description: "Log whether the family took a walk on a given date (default today), with an optional note.",
		Schema: obj(map[string]any{
			"walked": map[string]any{"type": "boolean", "description": "true if the family walked"},
			"date":   map[string]any{"type": "string", "description": "YYYY-MM-DD, defaults to today"},
			"note":   map[string]any{"type": "string", "description": "optional note"},
		}, "walked"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				Walked bool   `json:"walked"`
				Date   string `json:"date"`
				Note   string `json:"note"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", fmt.Errorf("bad arguments: %w", err)
			}
			if a.Date == "" {
				a.Date = time.Now().Format(dateFormat)
			}
			if _, err := time.Parse(dateFormat, a.Date); err != nil {
				return "", fmt.Errorf("date must be YYYY-MM-DD")
			}
			if err := setWalkDay(ctx, sqldb, user.Login, a.Date, a.Walked, strings.TrimSpace(a.Note)); err != nil {
				return "", err
			}
			web.Changed(user.Login)
			streak, err := currentStreak(ctx, sqldb, user.Login, time.Now().Format(dateFormat))
			if err != nil {
				return "", err
			}
			verb := "no walk"
			if a.Walked {
				verb = "walked"
			}
			return fmt.Sprintf("Logged %s for %s. Current streak: %d.", verb, a.Date, streak), nil
		},
	})

	web.Tool(mux, web.ToolDef{
		Name:        "check_streak",
		Description: "Report the current family walk streak and today's status.",
		Schema:      obj(map[string]any{}),
		Handler: func(ctx context.Context, user auth.User, _ json.RawMessage) (string, error) {
			today := time.Now().Format(dateFormat)
			walked, logged, err := todayStatus(ctx, sqldb, user.Login, today)
			if err != nil {
				return "", err
			}
			streak, err := currentStreak(ctx, sqldb, user.Login, today)
			if err != nil {
				return "", err
			}
			status := "not logged yet"
			if logged {
				status = map[bool]string{true: "yes", false: "no"}[walked]
			}
			msg := fmt.Sprintf("Current streak: %d nights. Today: %s.", streak, status)
			if m := milestoneMessage(streak); m != "" {
				msg += " " + m
			}
			return msg, nil
		},
	})
}

// chatContext gives in-app chat a short summary of recent status.
func chatContext(ctx context.Context, sqldb *sql.DB, login string) (string, error) {
	today := time.Now().Format(dateFormat)
	walked, logged, err := todayStatus(ctx, sqldb, login, today)
	if err != nil {
		return "", err
	}
	streak, err := currentStreak(ctx, sqldb, login, today)
	if err != nil {
		return "", err
	}
	status := "not logged yet"
	if logged {
		status = map[bool]string{true: "yes", false: "no"}[walked]
	}
	return fmt.Sprintf("Family Walks: today (%s) is %s. Current streak: %d nights.", today, status, streak), nil
}
