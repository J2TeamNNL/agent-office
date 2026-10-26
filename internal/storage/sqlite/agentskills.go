package sqlite

import (
	"context"
	"time"

	"github.com/tung1998/agent-office/internal/storage"
)

type agentSkillRepo struct{ db dbtx }

func (r agentSkillRepo) List(ctx context.Context, agentID string) ([]storage.AgentSkill, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT agent_id, name, why, source, uses, last_used_at, created_at FROM agent_skills WHERE agent_id=? ORDER BY created_at, name`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []storage.AgentSkill{}
	for rows.Next() {
		var (
			s             storage.AgentSkill
			used, created string
		)
		if err := rows.Scan(&s.AgentID, &s.Name, &s.Why, &s.Source, &s.Uses, &used, &created); err != nil {
			return nil, err
		}
		if err := parseTimes([]*time.Time{&s.CreatedAt}, created); err != nil {
			return nil, err
		}
		if used != "" {
			if err := parseTimes([]*time.Time{&s.LastUsedAt}, used); err != nil {
				return nil, err
			}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r agentSkillRepo) Add(ctx context.Context, s storage.AgentSkill) error {
	if s.Source == "" {
		s.Source = "person"
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO agent_skills (agent_id, name, why, source, created_at) VALUES (?,?,?,?,?)
		ON CONFLICT (agent_id, name) DO UPDATE SET why=CASE WHEN excluded.why='' THEN why ELSE excluded.why END`,
		s.AgentID, s.Name, s.Why, s.Source, fmtTime(time.Now()))
	return err
}

func (r agentSkillRepo) Remove(ctx context.Context, agentID, name string) error {
	return execOne(ctx, r.db, `DELETE FROM agent_skills WHERE agent_id=? AND name=?`, agentID, name)
}

func (r agentSkillRepo) Used(ctx context.Context, agentID, name string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE agent_skills SET uses=uses+1, last_used_at=? WHERE agent_id=? AND name=?`, fmtTime(time.Now()), agentID, name)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
