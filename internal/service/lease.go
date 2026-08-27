package service

import (
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

// LeaseRequest is a standalone lease acquisition.
type LeaseRequest struct {
	ResourceType domain.ResourceType `json:"resource_type"`
	ResourceID   string              `json:"resource_id"`
	Token        string              `json:"token"`
	HolderOp     string              `json:"holder_op"`
	AcquiredAt   domain.LogicalTime  `json:"acquired_at"`
	ExpiresAt    domain.LogicalTime  `json:"expires_at"`
}

// LeaseReleaseRequest frees a resource lease.
type LeaseReleaseRequest struct {
	ResourceType domain.ResourceType `json:"resource_type"`
	ResourceID   string              `json:"resource_id"`
	Token        string              `json:"token"`
	At           domain.LogicalTime  `json:"at"`
}

// AcquireLease grants a standalone resource lease.
func (s *Service) AcquireLease(req LeaseRequest) *domain.Error {
	return s.runTx(func(tx *store.Tx) *domain.Error {
		if err := tx.AcquireLease(domain.ResourceLease{
			ResourceType: req.ResourceType, ResourceID: req.ResourceID, Token: req.Token,
			HolderOp: req.HolderOp, AcquiredAt: req.AcquiredAt, ExpiresAt: req.ExpiresAt,
		}); err != nil {
			return txErr(err)
		}
		return nil
	})
}

// ReleaseLease frees a standalone resource lease.
func (s *Service) ReleaseLease(req LeaseReleaseRequest) *domain.Error {
	return s.runTx(func(tx *store.Tx) *domain.Error {
		if err := tx.ReleaseLease(req.ResourceType, req.ResourceID, req.Token, req.At); err != nil {
			return txErr(err)
		}
		return nil
	})
}
