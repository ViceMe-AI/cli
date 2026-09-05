package api

import "errors"

type WebsiteReplicaHostingProjection struct {
	Requested         bool                                `json:"requested"`
	Status            string                              `json:"status"`
	ActivePageRelease *WebsiteReplicaPageReleaseReference `json:"activePageRelease"`
	Repair            *WebsiteReplicaHostingRepairAction  `json:"repair"`
	LatestRepair      *WebsiteReplicaHostingLatestRepair  `json:"latestRepair"`
}
type WebsiteReplicaHostingRepairAction struct {
	Skill   string `json:"skill"`
	Command string `json:"command"`
}
type WebsiteReplicaHostingLatestRepair struct {
	ID          string                              `json:"id"`
	Status      string                              `json:"status"`
	Failure     *WebsiteReplicaPublicationFailure   `json:"failure"`
	PageRelease *WebsiteReplicaPageReleaseReference `json:"pageRelease"`
	CreatedAt   string                              `json:"createdAt"`
	UpdatedAt   string                              `json:"updatedAt"`
}

func validReplicaPageReference(r *WebsiteReplicaPageReleaseReference) bool {
	return r == nil || (zodUUIDPattern.MatchString(r.ID) && validPositiveSafeInteger(r.Version))
}
func (r *WebsiteReplicaPublication) validateHostingProjection() error {
	invalid := errors.New("invalid publication hosting or rollback projection")
	h := r.Hosting
	if r.PriceCents < 0 || r.PriceCents > 10_000_000 || !validStringEnum(h.Status, "NOT_REQUESTED", "PENDING", "ACTIVE", "DEGRADED", "REPAIRING") || !validReplicaPageReference(h.ActivePageRelease) {
		return invalid
	}
	if h.Repair != nil && (h.Repair.Skill != "$let-others-make-a-copy" || h.Repair.Command != "viceme replica repair-hosting --publication "+r.ID) {
		return invalid
	}
	if latest := h.LatestRepair; latest != nil {
		if !zodUUIDPattern.MatchString(latest.ID) || !validStringEnum(latest.Status, "WAITING_UPLOAD", "UPLOADED", "VALIDATING", "PUBLISHED", "FAILED") || !validZodDatetime(latest.CreatedAt) || !validZodDatetime(latest.UpdatedAt) || !validReplicaPageReference(latest.PageRelease) {
			return invalid
		}
	}
	if r.Rollback.AvailablePairs == nil {
		return invalid
	}
	if r.Rollback.ActivePair == nil {
		if len(r.Rollback.AvailablePairs) != 0 {
			return invalid
		}
	} else {
		if validateWebsiteReplicaVersionPair(*r.Rollback.ActivePair) != nil {
			return invalid
		}
		for _, pair := range r.Rollback.AvailablePairs {
			if pair.ID == r.Rollback.ActivePair.ID || validateWebsiteReplicaVersionPair(pair) != nil {
				return invalid
			}
		}
	}
	return nil
}
