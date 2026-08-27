package store

import (
	"database/sql"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// PostMass records a single double-entry mass ledger row after validating that
// the component balance never goes negative. It is atomic with the surrounding
// transaction, so a rejected overdraw rolls back every other pending change.
func (t *Tx) PostMass(entry domain.MassEntry) error {
	bal, err := t.massBalance(entry.Generation, entry.Component)
	if err != nil {
		return err
	}
	signed := int64(entry.Amount)
	if entry.Direction == domain.MassOutput {
		signed = -signed
	}
	next, ok := domain.Add64(bal, signed)
	if !ok {
		return domain.NewError(domain.CodeMaterialOverdraw, false,
			domain.Reason{Message: "balance overflow"})
	}
	if next < 0 {
		return domain.NewError(domain.CodeMaterialOverdraw, false,
			domain.Reason{Message: "component overdraw"})
	}
	_, err = t.tx.Exec(
		`INSERT INTO mass_entries(generation, component, direction, category, amount, evidence)
		 VALUES(?,?,?,?,?,?)`,
		int64(entry.Generation), int64(entry.Component), int64(entry.Direction),
		int64(entry.Category), int64(entry.Amount), entry.Evidence)
	return err
}

func (t *Tx) massBalance(gen domain.Generation, comp domain.Component) (int64, error) {
	var n sql.NullInt64
	err := t.tx.QueryRow(
		`SELECT COALESCE(SUM(CASE WHEN direction=? THEN amount ELSE -amount END),0)
		 FROM mass_entries WHERE generation=? AND component=?`,
		int64(domain.MassInput), int64(gen), int64(comp)).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n.Int64, nil
}

// MassBalance returns the net balance for a generation and component.
func (t *Tx) MassBalance(gen domain.Generation, comp domain.Component) (domain.Milligrams, error) {
	b, err := t.massBalance(gen, comp)
	if err != nil {
		return 0, err
	}
	return domain.Milligrams(b), nil
}

// MassConserved reports whether every component of a generation balances to
// zero: all inputs have been consumed, remaindered or recorded as loss.
func (t *Tx) MassConserved(gen domain.Generation) (bool, error) {
	for _, c := range []domain.Component{domain.ComponentBase, domain.ComponentCatalyst, domain.ComponentPrimer} {
		b, err := t.massBalance(gen, c)
		if err != nil {
			return false, err
		}
		if b != 0 {
			return false, nil
		}
	}
	return true, nil
}

// MassEntries returns the full ordered ledger for a generation.
func (t *Tx) MassEntries(gen domain.Generation) ([]domain.MassEntry, error) {
	rows, err := t.tx.Query(
		`SELECT seq, generation, component, direction, category, amount, evidence
		 FROM mass_entries WHERE generation=? ORDER BY seq ASC`, int64(gen))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MassEntry
	for rows.Next() {
		var e domain.MassEntry
		var seq, gen, comp, dir, cat, amt int64
		if err := rows.Scan(&seq, &gen, &comp, &dir, &cat, &amt, &e.Evidence); err != nil {
			return nil, err
		}
		e.Seq = seq
		e.Generation = domain.Generation(gen)
		e.Component = domain.Component(comp)
		e.Direction = domain.MassDirection(dir)
		e.Category = domain.MassCategory(cat)
		e.Amount = domain.Milligrams(amt)
		out = append(out, e)
	}
	return out, rows.Err()
}

// GenerationSeen reports whether any mass entry exists for the generation.
func (t *Tx) GenerationSeen(gen domain.Generation) (bool, error) {
	var one int
	err := t.tx.QueryRow(`SELECT 1 FROM mass_entries WHERE generation=? LIMIT 1`, int64(gen)).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// AllGenerations returns every distinct material generation that has mass
// entries, ordered ascending.
func (t *Tx) AllGenerations() ([]domain.Generation, error) {
	rows, err := t.tx.Query(`SELECT DISTINCT generation FROM mass_entries ORDER BY generation ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Generation
	for rows.Next() {
		var g int64
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, domain.Generation(g))
	}
	return out, rows.Err()
}
