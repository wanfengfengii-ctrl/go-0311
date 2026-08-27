package store

import (
	"database/sql"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// AcquireLease holds a resource exclusively until expiry. A conflicting
// unexpired lease held under a different token yields LEASE_CONFLICT; a
// re-acquire under the same token is an idempotent no-op.
func (t *Tx) AcquireLease(lease domain.ResourceLease) error {
	var cur domain.ResourceLease
	var found bool
	var acq, exp int64
	err := t.tx.QueryRow(
		`SELECT token, holder_op, acquired_at, expires_at FROM leases
		 WHERE resource_type=? AND resource_id=?`,
		int64(lease.ResourceType), lease.ResourceID).Scan(&cur.Token, &cur.HolderOp, &acq, &exp)
	switch {
	case err == sql.ErrNoRows:
		found = false
	case err != nil:
		return err
	default:
		cur.AcquiredAt = domain.LogicalTime(acq)
		cur.ExpiresAt = domain.LogicalTime(exp)
		found = true
	}
	if found && cur.ExpiresAt > lease.AcquiredAt && cur.Token != lease.Token {
		return domain.NewError(domain.CodeLeaseConflict, false,
			domain.Reason{Message: "resource already held"})
	}
	if found && cur.Token == lease.Token {
		return nil
	}
	_, err = t.tx.Exec(
		`INSERT INTO leases(resource_type, resource_id, token, holder_op, acquired_at, expires_at)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(resource_type, resource_id) DO UPDATE SET token=?, holder_op=?, acquired_at=?, expires_at=?`,
		int64(lease.ResourceType), lease.ResourceID, lease.Token, lease.HolderOp,
		int64(lease.AcquiredAt), int64(lease.ExpiresAt),
		lease.Token, lease.HolderOp, int64(lease.AcquiredAt), int64(lease.ExpiresAt))
	return err
}

// ReleaseLease frees a resource when the presented token matches and the lease
// has not yet expired.
func (t *Tx) ReleaseLease(resourceType domain.ResourceType, resourceID, token string, at domain.LogicalTime) error {
	res, err := t.tx.Exec(
		`DELETE FROM leases WHERE resource_type=? AND resource_id=? AND token=? AND expires_at>?`,
		int64(resourceType), resourceID, token, int64(at))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var cur domain.ResourceLease
		var exp int64
		err := t.tx.QueryRow(
			`SELECT token, expires_at FROM leases WHERE resource_type=? AND resource_id=?`,
			int64(resourceType), resourceID).Scan(&cur.Token, &exp)
		cur.ExpiresAt = domain.LogicalTime(exp)
		if err == sql.ErrNoRows {
			return domain.NewError(domain.CodeLeaseExpired, false,
				domain.Reason{Message: "lease absent"})
		}
		if err != nil {
			return err
		}
		if cur.Token != token {
			return domain.NewError(domain.CodeLeaseConflict, false,
				domain.Reason{Message: "token mismatch"})
		}
		return domain.NewError(domain.CodeLeaseExpired, false,
			domain.Reason{Message: "lease expired"})
	}
	return nil
}

// LeaseHolder returns the current valid token for a resource at the given time.
func (t *Tx) LeaseHolder(resourceType domain.ResourceType, resourceID string, at domain.LogicalTime) (string, bool, error) {
	var token string
	var exp int64
	err := t.tx.QueryRow(
		`SELECT token, expires_at FROM leases WHERE resource_type=? AND resource_id=?`,
		int64(resourceType), resourceID).Scan(&token, &exp)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if domain.LogicalTime(exp) <= at {
		return "", false, nil
	}
	return token, true, nil
}

// MaxLogicalTime returns the high-water mark of the business clock: the
// greatest logical_time recorded across every committed evidence event. It is
// the only clock domain in which leases are acquired and expired, so restart
// recovery must purge leases against this watermark rather than wall-clock
// time. With no events yet recorded it returns zero, leaving every lease (whose
// expiry is likewise a logical time) to be judged on its own merits.
func (t *Tx) MaxLogicalTime() (domain.LogicalTime, error) {
	var lt sql.NullInt64
	err := t.tx.QueryRow(`SELECT MAX(logical_time) FROM events`).Scan(&lt)
	if err != nil {
		return 0, err
	}
	if !lt.Valid {
		return 0, nil
	}
	return domain.LogicalTime(lt.Int64), nil
}

// ExpiredLeases purges leases whose expiry has passed at the given time. The
// watermark must be a logical time (the business clock), never wall-clock time:
// leases are acquired and expired in the logical-clock domain, so comparing a
// nanosecond wall-clock value here would delete every lease on restart. It is
// invoked by the restart-recovery bootstrap so stale holds do not block new
// work after a crash.
func (t *Tx) ExpiredLeases(at domain.LogicalTime) error {
	_, err := t.tx.Exec(`DELETE FROM leases WHERE expires_at<=?`, int64(at))
	return err
}
