package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/bketelsen/bespoke/pkg/auth"
)

// SkillDef is one bundled skill: procedural knowledge for chat, loaded on
// demand (ADR-0026).
type SkillDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

// Skills registers the app's bundled skills (ADR-0026): skillsFS holds
// <name>/SKILL.md files with `name:` and `description:` frontmatter — the
// same format as the repo's agent skills. It mounts GET /_skills (the
// parsed set, JSON) and registers one `load_skill` tool whose description
// carries the index, so every surface with the app's tools — its own chat,
// the dashboard chat (as <slug>_skill), external MCP clients — can load a
// skill's full instructions on demand. Call inside web.Run's register,
// before EnableChat.
func Skills(mux *http.ServeMux, skillsFS fs.FS) error {
	skills, err := parseSkills(skillsFS)
	if err != nil {
		return err
	}
	if len(skills) == 0 {
		return fmt.Errorf("no skills found (want <name>/SKILL.md)")
	}

	mux.HandleFunc("GET /_skills", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(skills)
	})

	byName := make(map[string]SkillDef, len(skills))
	names := make([]string, 0, len(skills))
	var index strings.Builder
	for _, s := range skills {
		byName[s.Name] = s
		names = append(names, s.Name)
		fmt.Fprintf(&index, "%s — %s; ", s.Name, s.Description)
	}

	Tool(mux, ToolDef{
		Name: "load_skill",
		Description: "Load one of this app's skills: detailed procedures for working with its data, " +
			"returned as markdown. Load the relevant skill BEFORE doing the kind of task it covers. " +
			"Available: " + strings.TrimSuffix(index.String(), "; "),
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "enum": names, "description": "The skill to load."},
			},
			"required": []string{"name"},
		},
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", fmt.Errorf("name is required")
			}
			s, ok := byName[a.Name]
			if !ok {
				return "", fmt.Errorf("no skill %q; available: %s", a.Name, strings.Join(names, ", "))
			}
			return s.Body, nil
		},
	})
	return nil
}

// parseSkills reads every <name>/SKILL.md, splitting the agent-skill
// frontmatter (`--- name/description ---`) from the markdown body. A
// missing name falls back to the directory name. Returned sorted by name
// so the tool description is stable.
func parseSkills(skillsFS fs.FS) ([]SkillDef, error) {
	matches, err := fs.Glob(skillsFS, "*/SKILL.md")
	if err != nil {
		return nil, err
	}
	var skills []SkillDef
	for _, m := range matches {
		raw, err := fs.ReadFile(skillsFS, m)
		if err != nil {
			return nil, err
		}
		s := SkillDef{Name: path.Dir(m)}
		body := string(raw)
		if rest, ok := strings.CutPrefix(body, "---\n"); ok {
			if front, after, ok := strings.Cut(rest, "\n---\n"); ok {
				for _, line := range strings.Split(front, "\n") {
					if v, ok := strings.CutPrefix(line, "name:"); ok {
						s.Name = strings.TrimSpace(v)
					}
					if v, ok := strings.CutPrefix(line, "description:"); ok {
						s.Description = strings.TrimSpace(v)
					}
				}
				body = after
			}
		}
		s.Body = strings.TrimSpace(body)
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}
