package copilot

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed skills/*.md
var skillFiles embed.FS

// Skill is a parsed skill definition loaded from an embedded .md file.
type Skill struct {
	Name        string
	Description string
	Triggers    []string // keyword phrases that auto-activate this skill
	Body        string   // full markdown instructions (everything after the frontmatter)
}

// SkillRegistry holds all loaded skills, indexed by name.
type SkillRegistry struct {
	skills []Skill
	byName map[string]Skill
}

// LoadSkills parses all embedded skill .md files and returns a registry.
func LoadSkills() (*SkillRegistry, error) {
	entries, err := skillFiles.ReadDir("skills")
	if err != nil {
		return nil, fmt.Errorf("copilot: read skills dir: %w", err)
	}
	reg := &SkillRegistry{byName: map[string]Skill{}}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := skillFiles.ReadFile("skills/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("copilot: read skill %s: %w", entry.Name(), err)
		}
		skill, err := parseSkill(string(data))
		if err != nil {
			return nil, fmt.Errorf("copilot: parse skill %s: %w", entry.Name(), err)
		}
		reg.skills = append(reg.skills, skill)
		reg.byName[skill.Name] = skill
	}
	return reg, nil
}

// List returns all skills.
func (r *SkillRegistry) List() []Skill { return r.skills }

// Get returns a skill by name.
func (r *SkillRegistry) Get(name string) (Skill, bool) {
	s, ok := r.byName[name]
	return s, ok
}

// CatalogText renders a compact skill catalog for the system prompt:
// one line per skill with name and description.
func (r *SkillRegistry) CatalogText() string {
	var b strings.Builder
	for _, s := range r.skills {
		b.WriteString("- **")
		b.WriteString(s.Name)
		b.WriteString("**: ")
		b.WriteString(s.Description)
		b.WriteByte('\n')
	}
	return b.String()
}

// InlineText renders the full instructions of the given skills for inclusion
// in the system prompt.
func InlineText(skills []Skill) string {
	var b strings.Builder
	for _, s := range skills {
		b.WriteString("### skill: ")
		b.WriteString(s.Name)
		b.WriteString("\n> ")
		b.WriteString(s.Description)
		b.WriteString("\n\n")
		b.WriteString(s.Body)
		b.WriteString("\n\n---\n\n")
	}
	return b.String()
}

// Match returns all skills whose triggers appear in the user message.
// Matching is case-insensitive. Returns nil if no skill matches.
func (r *SkillRegistry) Match(userMessage string) []Skill {
	if r == nil || len(r.skills) == 0 {
		return nil
	}
	lower := strings.ToLower(userMessage)
	var matched []Skill
	seen := map[string]bool{}
	for _, s := range r.skills {
		if seen[s.Name] {
			continue
		}
		for _, trigger := range s.Triggers {
			if strings.Contains(lower, trigger) {
				matched = append(matched, s)
				seen[s.Name] = true
				break
			}
		}
	}
	return matched
}

// parseSkill extracts YAML frontmatter (name, description) and the markdown
// body from a skill file. Frontmatter is delimited by --- lines.
func parseSkill(content string) (Skill, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return Skill{}, fmt.Errorf("missing frontmatter delimiter")
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return Skill{}, fmt.Errorf("missing closing frontmatter delimiter")
	}
	frontmatter := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:])

	var s Skill
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if colon := strings.Index(line, ":"); colon > 0 {
			key := strings.TrimSpace(line[:colon])
			val := strings.TrimSpace(line[colon+1:])
			val = strings.Trim(val, `"'`)
			switch key {
			case "name":
				s.Name = val
			case "description":
				s.Description = val
			case "triggers":
				for _, t := range strings.Split(val, ",") {
					t = strings.TrimSpace(strings.ToLower(t))
					if t != "" {
						s.Triggers = append(s.Triggers, t)
					}
				}
			}
		}
	}
	if s.Name == "" {
		return Skill{}, fmt.Errorf("skill missing 'name' in frontmatter")
	}
	if s.Description == "" {
		return Skill{}, fmt.Errorf("skill %q missing 'description' in frontmatter", s.Name)
	}
	s.Body = body
	return s, nil
}
