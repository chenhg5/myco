package domain

import (
	"strings"
)

// Skill is one kind of work an agent can be asked to do: what discovery and
// delegation pick agents by. Name is a short tag (it is also the agent's
// capability of that name); Summary says in a line what it covers;
// Examples are requests it fits.
type Skill struct {
	Name     string
	Summary  string
	Examples []string
}

const (
	maxSkills        = 20
	maxSkillName     = 40
	maxSkillSummary  = 200
	maxSkillExamples = 3
	maxSkillExample  = 200
)

// ParseSkills validates and normalizes skills: names are lowercase tags,
// unique; summaries and examples are single lines within their limits.
func ParseSkills(in []Skill) ([]Skill, error) {
	if len(in) > maxSkills {
		return nil, Errf(CodeInvalidArgument, "%d skills; at most %d", len(in), maxSkills).
			WithSuggestion("keep the skills others should pick this agent for")
	}
	out := make([]Skill, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		name := strings.ToLower(strings.TrimSpace(s.Name))
		if name == "" {
			return nil, Errf(CodeInvalidArgument, "a skill needs a name").
				WithSuggestion("names are short lowercase tags such as ios-release, code-review")
		}
		if len(name) > maxSkillName || !namePattern.MatchString(name) {
			return nil, Errf(CodeInvalidArgument, "invalid skill name %q", s.Name).
				WithSuggestion("names are short lowercase tags (letters, digits, - and _) such as ios-release")
		}
		if seen[name] {
			return nil, Errf(CodeInvalidArgument, "skill %q listed twice", name)
		}
		seen[name] = true
		summary := oneLine(s.Summary)
		if len(summary) > maxSkillSummary {
			return nil, Errf(CodeInvalidArgument, "summary of skill %q longer than %d bytes", name, maxSkillSummary)
		}
		var examples []string
		for _, e := range s.Examples {
			if e = oneLine(e); e == "" {
				continue
			}
			if len(e) > maxSkillExample {
				return nil, Errf(CodeInvalidArgument, "an example of skill %q is longer than %d bytes", name, maxSkillExample)
			}
			examples = append(examples, e)
		}
		if len(examples) > maxSkillExamples {
			return nil, Errf(CodeInvalidArgument, "skill %q has %d examples; at most %d", name, len(examples), maxSkillExamples)
		}
		out = append(out, Skill{Name: name, Summary: summary, Examples: examples})
	}
	return out, nil
}

// SkillNames lists the skills' names (the agent's capabilities).
func SkillNames(skills []Skill) []string {
	out := make([]string, 0, len(skills))
	for _, s := range skills {
		out = append(out, s.Name)
	}
	return out
}

// skillsFor returns skills matching caps: the ones already described keep
// their summary and examples, new names come bare.
func skillsFor(caps []string, have []Skill) []Skill {
	out := make([]Skill, 0, len(caps))
	for _, c := range caps {
		s := Skill{Name: c}
		for _, h := range have {
			if h.Name == c {
				s = h
				break
			}
		}
		out = append(out, s)
	}
	return out
}

// SetSkills replaces the agent's skills (and so its capabilities).
func (a *Agent) SetSkills(in []Skill) error {
	skills, err := ParseSkills(in)
	if err != nil {
		return err
	}
	a.Skills, a.Capabilities = skills, SkillNames(skills)
	return nil
}

// NormalizeSkills makes Skills and Capabilities agree after loading: an
// agent stored before skills existed gets one bare skill per capability.
func (a *Agent) NormalizeSkills() {
	if len(a.Skills) == 0 && len(a.Capabilities) > 0 {
		a.Skills = skillsFor(a.Capabilities, nil)
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
