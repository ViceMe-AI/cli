package api

import (
	"encoding/json"
	"errors"
)

const (
	WebsiteReplicaPublicationProtocolVersion = 2
	WebsiteReplicaPublicationConfirmationTTL = 30 * 60
)

type WebsiteReplicaPublicationSourceArtifact struct {
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
	Digest      string `json:"digest"`
}

type WebsiteReplicaPublicationTarget struct {
	Kind      string `json:"kind"`
	Slug      string `json:"slug,omitempty"`
	WorkID    string `json:"workId,omitempty"`
	ReplicaID string `json:"replicaId,omitempty"`
	ProductID string `json:"productId,omitempty"`
}

type WebsiteReplicaPublicationReview struct {
	AllowAutomaticDegradation bool                                     `json:"allowAutomaticDegradation"`
	Resolution                string                                   `json:"resolution"`
	MerchantAccountID         string                                   `json:"merchantAccountId"`
	MerchantDisplayName       string                                   `json:"merchantDisplayName"`
	CreatorAccountID          string                                   `json:"creatorAccountId"`
	CreatorHandle             string                                   `json:"creatorHandle"`
	CreatorDisplayName        string                                   `json:"creatorDisplayName"`
	ProjectFingerprint        string                                   `json:"projectFingerprint"`
	WorkURL                   string                                   `json:"workUrl"`
	CanonicalOrigin           *string                                  `json:"canonicalOrigin"`
	Title                     string                                   `json:"title"`
	Summary                   string                                   `json:"summary"`
	PriceCents                int                                      `json:"priceCents"`
	Source                    WebsiteReplicaPublicationSourceArtifact  `json:"source"`
	Page                      *WebsiteReplicaPublicationSourceArtifact `json:"page,omitempty" api:"optional"`
}

type WebsiteReplicaPublicationConfirmationChallenge struct {
	Version   string                          `json:"version"`
	Review    WebsiteReplicaPublicationReview `json:"review"`
	IssuedAt  string                          `json:"issuedAt"`
	ExpiresAt string                          `json:"expiresAt"`
}

type WebsiteReplicaPublicationConfirmation struct {
	Version     string                          `json:"version"`
	Review      WebsiteReplicaPublicationReview `json:"review"`
	IssuedAt    string                          `json:"issuedAt"`
	ExpiresAt   string                          `json:"expiresAt"`
	ConfirmedAt string                          `json:"confirmedAt"`
}

type CreateWebsiteReplicaPublicationRequest struct {
	AllowAutomaticDegradation bool                                     `json:"allowAutomaticDegradation"`
	ProtocolVersion           int                                      `json:"protocolVersion"`
	ClientRequestID           string                                   `json:"clientRequestId"`
	Market                    string                                   `json:"market"`
	MerchantAccountID         string                                   `json:"merchantAccountId,omitempty"`
	ProjectFingerprint        string                                   `json:"projectFingerprint"`
	Target                    WebsiteReplicaPublicationTarget          `json:"target"`
	CanonicalOrigin           *string                                  `json:"canonicalOrigin,omitempty"`
	Title                     string                                   `json:"title"`
	Summary                   string                                   `json:"summary"`
	PriceCents                int                                      `json:"priceCents"`
	Source                    WebsiteReplicaPublicationSourceArtifact  `json:"source"`
	Page                      *WebsiteReplicaPublicationSourceArtifact `json:"page,omitempty"`
	Confirmation              *WebsiteReplicaPublicationConfirmation   `json:"confirmation"`
}

type WebsiteReplicaPublicationMerchantChoice struct {
	ID            string `json:"id"`
	DisplayName   string `json:"displayName"`
	CreatorHandle string `json:"creatorHandle"`
}

type WebsiteReplicaPublicationSlugChoice struct {
	Slug    string `json:"slug"`
	WorkURL string `json:"workUrl"`
}

type WebsiteReplicaPublicationNextAction struct {
	Kind                   string                                          `json:"kind"`
	AuthURL                string                                          `json:"authUrl,omitempty"`
	ApplicationURL         string                                          `json:"applicationUrl,omitempty"`
	OnboardingID           string                                          `json:"onboardingId,omitempty"`
	StatusURL              string                                          `json:"statusUrl,omitempty"`
	Merchants              []WebsiteReplicaPublicationMerchantChoice       `json:"merchants,omitempty"`
	Candidates             []WebsiteReplicaPublicationSlugChoice           `json:"candidates,omitempty"`
	MinimumProtocolVersion int                                             `json:"minimumProtocolVersion,omitempty"`
	UpgradeURL             string                                          `json:"upgradeUrl,omitempty"`
	PublicationID          string                                          `json:"publicationId,omitempty"`
	Confirmation           *WebsiteReplicaPublicationConfirmationChallenge `json:"confirmation,omitempty"`
}

func (action *WebsiteReplicaPublicationNextAction) UnmarshalJSON(data []byte) error {
	var discriminator struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	*action = WebsiteReplicaPublicationNextAction{Kind: discriminator.Kind}
	switch discriminator.Kind {
	case "AUTHENTICATE_CREATOR":
		var value struct {
			Kind    string `json:"kind"`
			AuthURL string `json:"authUrl"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		action.AuthURL = value.AuthURL
	case "APPLY_CREATOR":
		var value struct {
			Kind           string `json:"kind"`
			ApplicationURL string `json:"applicationUrl"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		action.ApplicationURL = value.ApplicationURL
	case "WAIT_CREATOR_REVIEW", "SUPPLY_CREATOR_INFO", "CREATOR_APPLICATION_REJECTED":
		var value struct {
			Kind         string `json:"kind"`
			OnboardingID string `json:"onboardingId"`
			StatusURL    string `json:"statusUrl"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		action.OnboardingID, action.StatusURL = value.OnboardingID, value.StatusURL
	case "SELECT_MERCHANT":
		var value struct {
			Kind      string                                    `json:"kind"`
			Merchants []WebsiteReplicaPublicationMerchantChoice `json:"merchants"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		action.Merchants = value.Merchants
	case "CHOOSE_WORK_SLUG":
		var value struct {
			Kind       string                                `json:"kind"`
			Candidates []WebsiteReplicaPublicationSlugChoice `json:"candidates"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		action.Candidates = value.Candidates
	case "UPGRADE_CLI":
		var value struct {
			Kind                   string `json:"kind"`
			MinimumProtocolVersion int    `json:"minimumProtocolVersion"`
			UpgradeURL             string `json:"upgradeUrl"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		action.MinimumProtocolVersion, action.UpgradeURL = value.MinimumProtocolVersion, value.UpgradeURL
	case "CHECK_STATUS":
		var value struct {
			Kind          string `json:"kind"`
			PublicationID string `json:"publicationId"`
			StatusURL     string `json:"statusUrl"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		action.PublicationID, action.StatusURL = value.PublicationID, value.StatusURL
	case "AUTHORIZE_SOURCE_UPLOAD", "AUTHORIZE_PAGE_UPLOAD":
		var value struct {
			Kind          string `json:"kind"`
			PublicationID string `json:"publicationId"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		action.PublicationID = value.PublicationID
	case "CONFIRM_PUBLICATION":
		var value struct {
			Kind         string                                         `json:"kind"`
			Confirmation WebsiteReplicaPublicationConfirmationChallenge `json:"confirmation"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		action.Confirmation = &value.Confirmation
	default:
		return errors.New("Website Replica Publication next action is invalid")
	}
	return nil
}

type WebsiteReplicaPublicationResolvedTarget struct {
	Resolution        string  `json:"resolution"`
	MerchantAccountID string  `json:"merchantAccountId"`
	WorkID            string  `json:"workId"`
	ReplicaID         string  `json:"replicaId"`
	ProductID         *string `json:"productId"`
	WorkURL           string  `json:"workUrl"`
}

type CreateWebsiteReplicaPublicationResponse struct {
	Outcome         string                                   `json:"outcome"`
	ClientRequestID string                                   `json:"clientRequestId,omitempty"`
	Market          string                                   `json:"market,omitempty"`
	NextAction      WebsiteReplicaPublicationNextAction      `json:"nextAction"`
	Target          *WebsiteReplicaPublicationResolvedTarget `json:"target,omitempty"`
	Publication     *WebsiteReplicaPublication               `json:"publication,omitempty"`
}

func (response *CreateWebsiteReplicaPublicationResponse) UnmarshalJSON(data []byte) error {
	var discriminator struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	switch discriminator.Outcome {
	case "ACTION_REQUIRED":
		var value struct {
			Outcome         string                              `json:"outcome"`
			ClientRequestID string                              `json:"clientRequestId"`
			Market          string                              `json:"market"`
			NextAction      WebsiteReplicaPublicationNextAction `json:"nextAction"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		*response = CreateWebsiteReplicaPublicationResponse{
			Outcome: value.Outcome, ClientRequestID: value.ClientRequestID,
			Market: value.Market, NextAction: value.NextAction,
		}
	case "PUBLICATION_READY":
		var value struct {
			Outcome     string                                  `json:"outcome"`
			Target      WebsiteReplicaPublicationResolvedTarget `json:"target"`
			Publication WebsiteReplicaPublication               `json:"publication"`
			NextAction  WebsiteReplicaPublicationNextAction     `json:"nextAction"`
		}
		if err := decodeStrictObject(data, &value); err != nil {
			return err
		}
		*response = CreateWebsiteReplicaPublicationResponse{
			Outcome: value.Outcome, Target: &value.Target,
			Publication: &value.Publication, NextAction: value.NextAction,
		}
	default:
		return errors.New("Website Replica Publication create outcome is invalid")
	}
	return nil
}

type AuthorizeWebsiteReplicaPublicationSourceUploadResponse struct {
	PublicationID string                            `json:"publicationId"`
	Upload        WebsiteReplicaUploadAuthorization `json:"upload"`
}

type AuthorizeWebsiteReplicaPublicationPageUploadResponse struct {
	PublicationID string                            `json:"publicationId"`
	Upload        WebsiteReplicaUploadAuthorization `json:"upload"`
}

func decodeStrictObject(data []byte, target any) error {
	return decodeStrictAPIResponse(data, target)
}
