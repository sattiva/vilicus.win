package store

import (
	"context"
	"strings"
)


type CaseSearchHit struct {
	Src      string
	CaseID   int64
	GuildID  string
	CaseNo   int64
	Type     string
	TargetID string
	Snippet  string
}

func sanitizeFTSQuery(q string) string {
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' ||
			'0' <= r && r <= '9' || r == '_' || r == '-')
	})
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, "-")
		if f == "" {
			continue
		}
		terms = append(terms, `"`+f+`"`)
	}
	return strings.Join(terms, " ")
}

func (s *Store) SearchCases(ctx context.Context, q string, limit int) ([]CaseSearchHit, error) {
	match := sanitizeFTSQuery(q)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT f.src,
       COALESCE(c.id, n.case_id) AS case_id,
       COALESCE(c.guild_id, pc.guild_id),
       COALESCE(c.case_no, pc.case_no),
       COALESCE(c.type, pc.type),
       COALESCE(c.target_id, pc.target_id),
       snippet(cases_fts, 1, '', '', ' ... ', 10),
       snippet(cases_fts, 0, '', '', ' ... ', 10)
FROM cases_fts f
LEFT JOIN mod_cases c  ON f.src = 'case' AND c.id = f.rid
LEFT JOIN case_notes n ON f.src = 'note' AND n.id = f.rid
LEFT JOIN mod_cases pc ON n.case_id = pc.id
WHERE cases_fts MATCH ?
ORDER BY rank
LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CaseSearchHit
	for rows.Next() {
		var h CaseSearchHit
		var reasonSnip, bodySnip string
		var guildID, caseNo, caseType, targetID any
		if err := rows.Scan(&h.Src, &h.CaseID,
			&guildID, &caseNo, &caseType, &targetID,
			&reasonSnip, &bodySnip); err != nil {
			return nil, err
		}
		h.GuildID, _ = guildID.(string)
		h.Type, _ = caseType.(string)
		h.TargetID, _ = targetID.(string)
		if v, ok := caseNo.(int64); ok {
			h.CaseNo = v
		}
		h.Snippet = strings.TrimSpace(reasonSnip)
		if h.Snippet == "" {
			h.Snippet = strings.TrimSpace(bodySnip)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

