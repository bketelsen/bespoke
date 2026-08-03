package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/web"
)

func registerTools(mux *http.ServeMux, sqldb *sql.DB) {
	web.Tool(mux, web.ToolDef{
		Name: "add_note", Description: "Append a note to the user's notes stream.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}, "required": []string{"text"}},
		Handler: func(ctx context.Context, user auth.User, raw json.RawMessage) (string, error) {
			var args struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.Text) == "" {
				return "", fmt.Errorf("text is required")
			}
			if err := addNote(ctx, sqldb, user.Login, args.Text); err != nil {
				return "", err
			}
			return "Note saved.", nil
		},
	})
}
