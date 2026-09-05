package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"time"
)

const maxJSONSafeInteger = int64(1<<53 - 1)

const websiteReplicaMaxArtifactBytes = int64(100 * 1024 * 1024)

var (
	zodDatetimePattern               = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2})(?::(\d{2})(?:\.(\d+))?)?Z$`)
	zodUUIDPattern                   = regexp.MustCompile(`(?i)^(?:[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}|00000000-0000-0000-0000-000000000000|ffffffff-ffff-ffff-ffff-ffffffffffff)$`)
	jsonRawMessageType               = reflect.TypeOf(json.RawMessage{})
	replicaActionType                = reflect.TypeOf((*WebsiteReplicaPaymentAction)(nil))
	replicaPublicationNextActionType = reflect.TypeOf(WebsiteReplicaPublicationNextAction{})
)

type strictAPIResponse interface {
	strictAPIResponse()
}

func (*AuthorizeWebsiteReplicaPublicationSourceUploadResponse) strictAPIResponse() {}
func (*AuthorizeWebsiteReplicaPublicationPageUploadResponse) strictAPIResponse()   {}
func (*WebsiteReplicaPublication) strictAPIResponse()                              {}
func (*WebsiteReplicaResolution) strictAPIResponse()                               {}
func (*WebsiteReplicaSession) strictAPIResponse()                                  {}
func (*CheckoutWebsiteReplicaResponse) strictAPIResponse()                         {}
func (*WebsiteReplicaQuote) strictAPIResponse()                                    {}
func (*WebsiteReplicaOrder) strictAPIResponse()                                    {}
func (*WebsiteReplicaOrderStatus) strictAPIResponse()                              {}
func (*WebsiteReplicaDownload) strictAPIResponse()                                 {}
func (*WebsiteReplicaInstallationReceipt) strictAPIResponse()                      {}
func (*WebsiteReplicaRollback) strictAPIResponse()                                 {}

func decodeStrictAPIResponse(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("response contains trailing JSON values")
	}
	var raw any
	rawDecoder := json.NewDecoder(bytes.NewReader(data))
	rawDecoder.UseNumber()
	if err := rawDecoder.Decode(&raw); err != nil {
		return err
	}
	targetType := reflect.TypeOf(target)
	if targetType == nil || targetType.Kind() != reflect.Pointer {
		return errors.New("strict response target must be a pointer")
	}
	return requireJSONFields(targetType.Elem(), raw, "$")
}

func requireJSONFields(targetType reflect.Type, raw any, path string) error {
	if targetType == jsonRawMessageType {
		return nil
	}
	if targetType == replicaActionType {
		return nil
	}
	if targetType == replicaPublicationNextActionType {
		return nil
	}
	if targetType.Kind() == reflect.Pointer {
		if raw == nil {
			return nil
		}
		return requireJSONFields(targetType.Elem(), raw, path)
	}
	if raw == nil {
		return fmt.Errorf("%s must not be null", path)
	}
	switch targetType.Kind() {
	case reflect.Struct:
		object, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		allowedFields := make(map[string]struct{}, targetType.NumField())
		for index := 0; index < targetType.NumField(); index++ {
			field := targetType.Field(index)
			if !field.IsExported() {
				continue
			}
			tag := strings.Split(field.Tag.Get("json"), ",")
			name := tag[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			allowedFields[name] = struct{}{}
			value, found := object[name]
			// JSON omission on output does not imply an optional API field.
			// Only explicitly marked Zod optional fields may be absent.
			optional := field.Tag.Get("api") == "optional"
			if !found && optional {
				continue
			}
			if !found {
				return fmt.Errorf("%s.%s is required", path, name)
			}
			if optional && value == nil {
				return fmt.Errorf("%s.%s must not be null", path, name)
			}
			if err := requireJSONFields(field.Type, value, path+"."+name); err != nil {
				return err
			}
		}
		for name := range object {
			if _, allowed := allowedFields[name]; !allowed {
				return fmt.Errorf("%s.%s is not allowed", path, name)
			}
		}
	case reflect.Slice, reflect.Array:
		items, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		for index, item := range items {
			if err := requireJSONFields(targetType.Elem(), item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		object, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		if targetType.Elem().Kind() != reflect.Interface {
			for key, value := range object {
				if err := requireJSONFields(targetType.Elem(), value, path+"."+key); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (action *WebsiteReplicaPaymentAction) UnmarshalJSON(data []byte) error {
	var discriminator struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	switch discriminator.Type {
	case "QR_CODE":
		var value struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		}
		if err := decodeStrictAPIResponse(data, &value); err != nil {
			return err
		}
		*action = WebsiteReplicaPaymentAction{Type: value.Type, Content: value.Content}
	default:
		return errors.New("Website Replica payment action type is invalid")
	}
	return nil
}

func (response *WebsiteReplicaPublication) validateAPIResponse() error {
	if response != nil {
		if err := response.validateHostingProjection(); err != nil {
			return err
		}
	}
	if response == nil || !zodUUIDPattern.MatchString(response.ID) || !zodUUIDPattern.MatchString(response.ClientRequestID) ||
		(response.Market != "CN" && response.Market != "GLOBAL") || !zodUUIDPattern.MatchString(response.MerchantAccountID) ||
		!zodUUIDPattern.MatchString(response.WorkID) || !zodUUIDPattern.MatchString(response.ReplicaID) ||
		!validStringEnum(response.Status, "DRAFT", "PROCESSING", "PUBLISHED", "PUBLISHED_DEGRADED", "FAILED", "CANCELLED") ||
		!validAbsoluteURL(response.StatusURL) || response.AllowedActions == nil ||
		response.Retry.AutomaticRetries < 0 || response.Retry.AutomaticRetries > 3 || response.Retry.MaxAutomaticRetries != 3 ||
		!validOptionalDatetime(response.Retry.NextAttemptAt) || validateWebsiteReplicaPublicationSource(response.Source) != nil ||
		(response.Page != nil && validateWebsiteReplicaPublicationSource(*response.Page) != nil) ||
		!validOptionalDatetime(response.SubmittedAt) || !validOptionalDatetime(response.FailedAt) ||
		!validOptionalDatetime(response.CancelledAt) || !validZodDatetime(response.CreatedAt) || !validZodDatetime(response.UpdatedAt) {
		return errors.New("Website Replica Publication response is invalid")
	}
	actions := make(map[string]struct{}, len(response.AllowedActions))
	for _, action := range response.AllowedActions {
		if !validStringEnum(action, "AUTHORIZE_SOURCE_UPLOAD", "COMPLETE_SOURCE_UPLOAD", "AUTHORIZE_PAGE_UPLOAD", "COMPLETE_PAGE_UPLOAD", "SUBMIT", "CANCEL", "RETRY") {
			return errors.New("Website Replica Publication action is invalid")
		}
		if _, duplicate := actions[action]; duplicate {
			return errors.New("Website Replica Publication actions are not unique")
		}
		actions[action] = struct{}{}
	}
	verified := response.Source.Status == "VERIFIED" || response.Source.Status == "ACTIVATED"
	if verified != (response.Source.VerifiedAt != nil) {
		return errors.New("Website Replica Publication source verification timestamp is invalid")
	}
	published := response.Status == "PUBLISHED" || response.Status == "PUBLISHED_DEGRADED"
	pageVerified := response.Page == nil || response.Page.Status == "VERIFIED" || response.Page.Status == "ACTIVATED"
	pageReady := pageVerified || (response.Page != nil && response.Page.Status == "FAILED")
	pageActivated := response.Page == nil || response.Page.Status == "ACTIVATED"
	if published != (response.Result != nil) || (response.Result != nil && validateWebsiteReplicaPublicationResult(*response.Result) != nil) ||
		(published && response.Source.Status != "ACTIVATED") ||
		(response.Status == "PUBLISHED" && !pageActivated) ||
		(response.Status == "PUBLISHED" && response.Result != nil && ((response.Page == nil) != (response.Result.PageRelease == nil))) ||
		(response.Status == "PUBLISHED_DEGRADED" && response.Result != nil && response.Result.PageRelease != nil) ||
		(response.Status == "PUBLISHED_DEGRADED" && (response.Page == nil || response.Page.Status != "FAILED")) ||
		(response.Status == "PROCESSING" && (response.Source.Status != "VERIFIED" || !pageReady)) {
		return errors.New("Website Replica Publication result is invalid")
	}
	if (response.Status == "PUBLISHED" && response.Failure != nil) ||
		(response.Status == "PROCESSING" && response.Failure != nil && (response.Page == nil || response.Page.Status != "FAILED")) ||
		((response.Status == "FAILED" || response.Status == "PUBLISHED_DEGRADED") && response.Failure == nil) ||
		(response.Failure != nil && (utf16CodeUnits(strings.TrimSpace(response.Failure.Code)) < 1 || utf16CodeUnits(strings.TrimSpace(response.Failure.Code)) > 64 ||
			utf16CodeUnits(strings.TrimSpace(response.Failure.Message)) < 1 || utf16CodeUnits(strings.TrimSpace(response.Failure.Message)) > 500)) {
		return errors.New("Website Replica Publication failure is invalid")
	}
	submitted := response.Status == "PROCESSING" || response.Status == "PUBLISHED" || response.Status == "PUBLISHED_DEGRADED" || response.Status == "FAILED"
	if (submitted && response.SubmittedAt == nil) || (response.Status == "DRAFT" && response.SubmittedAt != nil) ||
		(response.Status == "FAILED") != (response.FailedAt != nil) ||
		(response.Status == "CANCELLED") != (response.CancelledAt != nil) ||
		(response.Retry.NextAttemptAt != nil && response.Status != "PROCESSING") {
		return errors.New("Website Replica Publication lifecycle timestamps are invalid")
	}
	_, canCancel := actions["CANCEL"]
	_, canRetry := actions["RETRY"]
	_, canAuthorize := actions["AUTHORIZE_SOURCE_UPLOAD"]
	_, canComplete := actions["COMPLETE_SOURCE_UPLOAD"]
	_, canAuthorizePage := actions["AUTHORIZE_PAGE_UPLOAD"]
	_, canCompletePage := actions["COMPLETE_PAGE_UPLOAD"]
	_, canSubmit := actions["SUBMIT"]
	if canCancel != (response.Status == "DRAFT" || response.Status == "PROCESSING") ||
		canRetry != (response.Status == "FAILED" && response.Failure != nil && response.Failure.Retryable) ||
		canAuthorize != (response.Status == "DRAFT" && response.Source.Status == "WAITING_UPLOAD") ||
		canComplete != (response.Status == "DRAFT" && validStringEnum(response.Source.Status, "WAITING_UPLOAD", "UPLOADED", "VALIDATING")) ||
		canAuthorizePage != (response.Status == "DRAFT" && response.Page != nil && response.Page.Status == "WAITING_UPLOAD") ||
		canCompletePage != (response.Status == "DRAFT" && response.Page != nil && validStringEnum(response.Page.Status, "WAITING_UPLOAD", "UPLOADED", "VALIDATING")) ||
		canSubmit != (response.Status == "DRAFT" && response.Source.Status == "VERIFIED" && pageReady) {
		return errors.New("Website Replica Publication allowed actions do not match its state")
	}
	return nil
}

func validateWebsiteReplicaPublicationResult(result WebsiteReplicaPublicationResult) error {
	if !validAbsoluteURL(result.WorkURL) || !zodUUIDPattern.MatchString(result.VersionID) || !validPositiveSafeInteger(result.Version) ||
		!websiteReplicaCodePattern.MatchString(result.ShortCode) || result.Instruction != "VICEME-REPLICA:"+result.ShortCode ||
		validateWebsiteReplicaProduct(result.Product) != nil ||
		(result.PageRelease != nil && (!zodUUIDPattern.MatchString(result.PageRelease.ID) || !validPositiveSafeInteger(result.PageRelease.Version))) ||
		!validZodDatetime(result.PublishedAt) {
		return errors.New("Website Replica Publication result is invalid")
	}
	return nil
}

func (response *CreateWebsiteReplicaPublicationResponse) validateAPIResponse() error {
	if response == nil || !validStringEnum(response.Outcome, "ACTION_REQUIRED", "PUBLICATION_READY") ||
		validateWebsiteReplicaPublicationNextAction(response.NextAction) != nil {
		return errors.New("Website Replica Publication create response is invalid")
	}
	if response.Outcome == "ACTION_REQUIRED" {
		if !zodUUIDPattern.MatchString(response.ClientRequestID) ||
			!validStringEnum(response.Market, "CN", "GLOBAL") || response.Target != nil || response.Publication != nil {
			return errors.New("Website Replica Publication action response is invalid")
		}
		return nil
	}
	if response.ClientRequestID != "" || response.Market != "" || response.Target == nil || response.Publication == nil ||
		validateWebsiteReplicaPublicationTarget(*response.Target) != nil || response.Publication.validateAPIResponse() != nil ||
		response.Publication.ID != response.NextAction.PublicationID || response.Publication.WorkID != response.Target.WorkID ||
		response.Publication.ReplicaID != response.Target.ReplicaID ||
		response.Publication.MerchantAccountID != response.Target.MerchantAccountID ||
		(response.Publication.Result != nil && (response.Target.ProductID == nil ||
			response.Publication.Result.Product.ID != *response.Target.ProductID || response.Publication.Result.WorkURL != response.Target.WorkURL)) ||
		!validStringEnum(response.NextAction.Kind, "AUTHORIZE_SOURCE_UPLOAD", "CHECK_STATUS") {
		return errors.New("Website Replica Publication ready response is invalid")
	}
	return nil
}

func (response *AuthorizeWebsiteReplicaPublicationSourceUploadResponse) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.PublicationID) || response.Upload.Method != "PUT" ||
		!validAbsoluteURL(response.Upload.URL) || response.Upload.Headers == nil || !validZodDatetime(response.Upload.ExpiresAt) {
		return errors.New("Website Replica Publication upload authorization is invalid")
	}
	return nil
}

func (response *AuthorizeWebsiteReplicaPublicationPageUploadResponse) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.PublicationID) || response.Upload.Method != "PUT" ||
		!validAbsoluteURL(response.Upload.URL) || response.Upload.Headers == nil || !validZodDatetime(response.Upload.ExpiresAt) {
		return errors.New("Website Replica Publication page upload authorization is invalid")
	}
	return nil
}

func validateWebsiteReplicaPublicationTarget(target WebsiteReplicaPublicationResolvedTarget) error {
	if !validStringEnum(target.Resolution, "CREATE", "UPDATE") || !zodUUIDPattern.MatchString(target.MerchantAccountID) ||
		!zodUUIDPattern.MatchString(target.WorkID) || !zodUUIDPattern.MatchString(target.ReplicaID) ||
		!validOptionalUUID(target.ProductID) || !validAbsoluteURL(target.WorkURL) {
		return errors.New("Website Replica Publication target is invalid")
	}
	return nil
}

func validateWebsiteReplicaPublicationNextAction(action WebsiteReplicaPublicationNextAction) error {
	switch action.Kind {
	case "AUTHENTICATE_CREATOR":
		if !validAbsoluteURL(action.AuthURL) {
			return errors.New("Website Replica authentication action is invalid")
		}
	case "APPLY_CREATOR":
		if !validAbsoluteURL(action.ApplicationURL) {
			return errors.New("Website Replica creator application action is invalid")
		}
	case "WAIT_CREATOR_REVIEW", "SUPPLY_CREATOR_INFO", "CREATOR_APPLICATION_REJECTED":
		if !zodUUIDPattern.MatchString(action.OnboardingID) || !validAbsoluteURL(action.StatusURL) {
			return errors.New("Website Replica creator review action is invalid")
		}
	case "SELECT_MERCHANT":
		if len(action.Merchants) == 0 {
			return errors.New("Website Replica Merchant selection is empty")
		}
		for _, merchant := range action.Merchants {
			if !zodUUIDPattern.MatchString(merchant.ID) || strings.TrimSpace(merchant.DisplayName) == "" ||
				utf16CodeUnits(strings.TrimSpace(merchant.CreatorHandle)) < 2 || utf16CodeUnits(strings.TrimSpace(merchant.CreatorHandle)) > 32 {
				return errors.New("Website Replica Merchant selection is invalid")
			}
		}
	case "CHOOSE_WORK_SLUG":
		if len(action.Candidates) == 0 {
			return errors.New("Website Replica Work slug selection is empty")
		}
		for _, candidate := range action.Candidates {
			if !validWebsiteReplicaWorkSlug(candidate.Slug) || !validAbsoluteURL(candidate.WorkURL) {
				return errors.New("Website Replica Work slug selection is invalid")
			}
		}
	case "UPGRADE_CLI":
		if action.MinimumProtocolVersion != WebsiteReplicaPublicationProtocolVersion || !validAbsoluteURL(action.UpgradeURL) {
			return errors.New("Website Replica CLI upgrade action is invalid")
		}
	case "CHECK_STATUS":
		if !zodUUIDPattern.MatchString(action.PublicationID) || !validAbsoluteURL(action.StatusURL) {
			return errors.New("Website Replica status action is invalid")
		}
	case "AUTHORIZE_SOURCE_UPLOAD", "AUTHORIZE_PAGE_UPLOAD":
		if !zodUUIDPattern.MatchString(action.PublicationID) {
			return errors.New("Website Replica upload action is invalid")
		}
	case "CONFIRM_PUBLICATION":
		if action.Confirmation == nil || validateWebsiteReplicaPublicationConfirmation(*action.Confirmation) != nil {
			return errors.New("Website Replica confirmation action is invalid")
		}
	default:
		return errors.New("Website Replica Publication action kind is invalid")
	}
	return nil
}

func validateWebsiteReplicaPublicationConfirmation(confirmation WebsiteReplicaPublicationConfirmationChallenge) error {
	if !regexp.MustCompile(`^wrv1-[a-f0-9]{64}$`).MatchString(confirmation.Version) ||
		!validZodDatetime(confirmation.IssuedAt) || !validZodDatetime(confirmation.ExpiresAt) ||
		validateWebsiteReplicaPublicationReview(confirmation.Review) != nil {
		return errors.New("Website Replica Publication confirmation is invalid")
	}
	issuedAt, issuedErr := parseZodDatetime(confirmation.IssuedAt)
	expiresAt, expiresErr := parseZodDatetime(confirmation.ExpiresAt)
	if issuedErr != nil || expiresErr != nil || expiresAt.Sub(issuedAt) != WebsiteReplicaPublicationConfirmationTTL*time.Second {
		return errors.New("Website Replica Publication confirmation TTL is invalid")
	}
	return nil
}

func validateWebsiteReplicaPublicationReview(review WebsiteReplicaPublicationReview) error {
	if !validStringEnum(review.Resolution, "CREATE", "UPDATE") || !zodUUIDPattern.MatchString(review.MerchantAccountID) ||
		strings.TrimSpace(review.MerchantDisplayName) == "" || utf16CodeUnits(strings.TrimSpace(review.MerchantDisplayName)) > 120 ||
		!zodUUIDPattern.MatchString(review.CreatorAccountID) || utf16CodeUnits(strings.TrimSpace(review.CreatorHandle)) < 2 ||
		utf16CodeUnits(strings.TrimSpace(review.CreatorHandle)) > 32 || strings.TrimSpace(review.CreatorDisplayName) == "" ||
		utf16CodeUnits(strings.TrimSpace(review.CreatorDisplayName)) > 120 || !sha256HexPattern.MatchString(review.ProjectFingerprint) ||
		!validAbsoluteURL(review.WorkURL) || (review.CanonicalOrigin != nil && !validHTTPSURL(*review.CanonicalOrigin)) ||
		utf16CodeUnits(strings.TrimSpace(review.Title)) < 1 || utf16CodeUnits(strings.TrimSpace(review.Title)) > 200 ||
		utf16CodeUnits(strings.TrimSpace(review.Summary)) > 500 || !validNonnegativeSafeInteger(review.PriceCents) ||
		review.PriceCents > 10_000_000 || validateWebsiteReplicaPublicationSourceArtifact(review.Source) != nil {
		return errors.New("Website Replica Publication review is invalid")
	}
	if review.AllowAutomaticDegradation && review.Page == nil {
		return errors.New("automatic degradation requires a requested hosted page")
	}
	if review.Page != nil && validateWebsiteReplicaPublicationSourceArtifact(*review.Page) != nil {
		return errors.New("Website Replica Publication review page artifact is invalid")
	}
	return nil
}

func validateWebsiteReplicaPublicationSourceArtifact(source WebsiteReplicaPublicationSourceArtifact) error {
	if utf16CodeUnits(strings.TrimSpace(source.FileName)) < 1 || utf16CodeUnits(strings.TrimSpace(source.FileName)) > 255 ||
		strings.ContainsAny(source.FileName, "/\\\x00") || !strings.HasSuffix(strings.ToLower(source.FileName), ".zip") ||
		source.ContentType != "application/zip" || source.SizeBytes < 1 || source.SizeBytes > websiteReplicaMaxArtifactBytes ||
		!sha256HexPattern.MatchString(source.Digest) {
		return errors.New("Website Replica Publication source artifact is invalid")
	}
	return nil
}

func validWebsiteReplicaWorkSlug(value string) bool {
	if len(value) < 2 || len(value) > 64 || !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(value) {
		return false
	}
	return !validStringEnum(value, "works", "skills", "manage", "posts", "about")
}

func validHTTPSURL(value string) bool {
	parsed, err := shopURLParser.Parse(value)
	return err == nil && parsed != nil && parsed.Scheme() == "https" && parsed.Host() != ""
}

func validateWebsiteReplicaPublicationSource(source WebsiteReplicaPublicationSource) error {
	if utf16CodeUnits(source.FileName) < 1 || utf16CodeUnits(source.FileName) > 255 || source.ContentType != "application/zip" ||
		strings.ContainsAny(source.FileName, "/\\\x00") || !strings.HasSuffix(strings.ToLower(source.FileName), ".zip") ||
		source.SizeBytes < 1 || source.SizeBytes > websiteReplicaMaxArtifactBytes || !sha256HexPattern.MatchString(source.Digest) ||
		!validStringEnum(source.Status, "WAITING_UPLOAD", "UPLOADED", "VALIDATING", "VERIFIED", "ACTIVATED", "FAILED") ||
		!validOptionalDatetime(source.VerifiedAt) {
		return errors.New("Website Replica Publication source is invalid")
	}
	return nil
}

func validWebsiteReplicaBuyerEntry(entry WebsiteReplicaBuyerEntry, instruction string) bool {
	return entry.Instruction == instruction && utf16CodeUnits(entry.Prompts.ZH) >= 1 && utf16CodeUnits(entry.Prompts.ZH) <= 2000 &&
		utf16CodeUnits(entry.Prompts.EN) >= 1 && utf16CodeUnits(entry.Prompts.EN) <= 2000 && validAbsoluteURL(entry.ViceMeWorkURL)
}

func (response *WebsiteReplicaResolution) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.ReplicaID) || !websiteReplicaCodePattern.MatchString(response.ShortCode) ||
		utf16CodeUnits(response.Title) < 1 || utf16CodeUnits(response.Title) > 200 || utf16CodeUnits(response.Summary) > 500 ||
		utf16CodeUnits(response.Creator.Handle) < 2 || utf16CodeUnits(response.Creator.Handle) > 32 ||
		!validAbsoluteURL(response.ViceMeWorkURL) ||
		validateWebsiteReplicaProduct(response.Product) != nil {
		return errors.New("Website Replica resolution response is invalid")
	}
	return nil
}

func (response *WebsiteReplicaSession) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.SessionID) ||
		len(response.Token) < 43 || len(response.Token) > 256 || !validZodDatetime(response.ExpiresAt) ||
		response.Replica.validateAPIResponse() != nil {
		return errors.New("Website Replica session response is invalid")
	}
	return nil
}

func (response *CheckoutWebsiteReplicaResponse) validateAPIResponse() error {
	if response == nil || !validAbsoluteURL(response.CheckoutURL) {
		return errors.New("Website Replica checkout response is invalid")
	}
	order := WebsiteReplicaOrder{
		OrderNo: response.OrderNo, Status: response.Status, PaymentAction: response.PaymentAction, ExpiresAt: response.ExpiresAt,
	}
	return order.validateAPIResponse()
}

func validateWebsiteReplicaProduct(product WebsiteReplicaProduct) error {
	if !zodUUIDPattern.MatchString(product.ID) || !zodUUIDPattern.MatchString(product.SKUID) || utf16CodeUnits(product.Title) < 1 ||
		utf16CodeUnits(product.Title) > 200 || !validReplicaCurrency(product.Currency) || !validNonnegativeSafeInteger(product.PriceCents) {
		return errors.New("Website Replica product is invalid")
	}
	return nil
}

func (response *WebsiteReplicaPublicationRollbackState) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.ID) ||
		(response.Status != "PUBLISHED" && response.Status != "PUBLISHED_DEGRADED") ||
		response.Rollback.ActivePair == nil || response.Rollback.AvailablePairs == nil ||
		validateWebsiteReplicaVersionPair(*response.Rollback.ActivePair) != nil {
		return errors.New("Website Replica rollback state is invalid")
	}
	seen := map[string]bool{response.Rollback.ActivePair.ID: true}
	for _, pair := range response.Rollback.AvailablePairs {
		if validateWebsiteReplicaVersionPair(pair) != nil || seen[pair.ID] {
			return errors.New("Website Replica rollback targets are invalid")
		}
		seen[pair.ID] = true
	}
	return nil
}

func (response *WebsiteReplicaRollback) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.ID) ||
		!zodUUIDPattern.MatchString(response.PublicationID) || !zodUUIDPattern.MatchString(response.ClientRequestID) ||
		validateWebsiteReplicaVersionPair(response.PreviousPair) != nil || validateWebsiteReplicaVersionPair(response.ActivePair) != nil ||
		response.PreviousPair.ID == response.ActivePair.ID || validateWebsiteReplicaProduct(response.Product) != nil ||
		!response.PriceUnchanged || !validZodDatetime(response.RolledBackAt) {
		return errors.New("Website Replica rollback response is invalid")
	}
	return nil
}

func validateWebsiteReplicaVersionPair(pair WebsiteReplicaVersionPair) error {
	if !zodUUIDPattern.MatchString(pair.ID) || !zodUUIDPattern.MatchString(pair.ReplicaVersion.ID) ||
		!validPositiveSafeInteger(pair.ReplicaVersion.Version) || !zodUUIDPattern.MatchString(pair.WorkRevisionID) ||
		!validZodDatetime(pair.CreatedAt) {
		return errors.New("Website Replica version pair is invalid")
	}
	if pair.PageRelease != nil && (!zodUUIDPattern.MatchString(pair.PageRelease.ID) || !validPositiveSafeInteger(pair.PageRelease.Version)) {
		return errors.New("Website Replica page release is invalid")
	}
	return nil
}

func (response *WebsiteReplicaQuote) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.ID) || !zodUUIDPattern.MatchString(response.Product.ID) ||
		!zodUUIDPattern.MatchString(response.Attribution.SubjectWorkID) || !validOptionalUUID(response.Attribution.EntryWorkID) ||
		!validOptionalUUID(response.Attribution.CommerceApplicationID) || !zodUUIDPattern.MatchString(response.SKU.ID) ||
		response.SKU.SelectedOptions == nil || !validReplicaCurrency(response.Currency) ||
		!validNonnegativeSafeInteger(response.UnitAmountCents) || !validPositiveSafeInteger(response.Quantity) ||
		!validNonnegativeSafeInteger(response.SubtotalAmountCents) || !validNonnegativeSafeInteger(response.ShippingAmountCents) ||
		!validNonnegativeSafeInteger(response.TotalAmountCents) || response.ContractSummary.PublicFields == nil ||
		response.ContractSummary.SensitiveFieldKeys == nil || !validNonnegativeSafeInteger(response.ContractSummary.AssetCount) ||
		response.Fulfillment.Capabilities == nil || response.PaymentOptions == nil || len(response.PaymentOptions) == 0 ||
		!validZodDatetime(response.ExpiresAt) || !validWebsiteReplicaQuoteSemantics(response) {
		return errors.New("Website Replica quote response is invalid")
	}
	for _, option := range response.PaymentOptions {
		if !validReplicaPaymentOption(option) {
			return errors.New("Website Replica quote payment option is invalid")
		}
	}
	return nil
}

func validWebsiteReplicaQuoteSemantics(response *WebsiteReplicaQuote) bool {
	return response.Quantity == 1 &&
		response.SubtotalAmountCents == response.UnitAmountCents &&
		response.ShippingAmountCents == 0 &&
		response.TotalAmountCents == response.SubtotalAmountCents &&
		len(response.ContractSummary.PublicFields) == 0 &&
		len(response.ContractSummary.SensitiveFieldKeys) == 0 &&
		response.ContractSummary.AssetCount == 0 &&
		len(response.Fulfillment.Capabilities) == 1 &&
		response.Fulfillment.Capabilities[0] == "DIGITAL_ENTITLEMENT" &&
		response.Fulfillment.EstimatedState == "AWAITING_PAYMENT" &&
		len(response.PaymentOptions) == 1 &&
		((response.TotalAmountCents == 0 && response.PaymentOptions[0].Provider == "FREE" && len(response.PaymentOptions[0].Scenes) == 0) ||
			(response.TotalAmountCents > 0 && response.PaymentOptions[0].Provider == "WECHAT_PAY" && len(response.PaymentOptions[0].Scenes) == 1 && response.PaymentOptions[0].Scenes[0] == "NATIVE"))
}

func (response *WebsiteReplicaOrder) validateAPIResponse() error {
	if response == nil || utf16CodeUnits(response.OrderNo) < 6 || utf16CodeUnits(response.OrderNo) > 40 ||
		!validWebsiteReplicaOrderStatus(response.Status) || !validZodDatetime(response.ExpiresAt) {
		return errors.New("Website Replica order response is invalid")
	}
	if response.Status == "PENDING" {
		if !validWebsiteReplicaPaymentAction(response.PaymentAction) {
			return errors.New("pending Website Replica order is missing its payment action")
		}
	} else if response.PaymentAction != nil {
		return errors.New("terminal Website Replica order exposed a payment action")
	}
	return nil
}

func (response *WebsiteReplicaOrderStatus) validateAPIResponse() error {
	if response == nil || utf16CodeUnits(response.OrderNo) < 6 || utf16CodeUnits(response.OrderNo) > 40 ||
		!validWebsiteReplicaOrderStatus(response.Payment.Status) || !validOptionalDatetime(response.Payment.PaidAt) ||
		!validOptionalDatetime(response.Payment.ClosedAt) {
		return errors.New("Website Replica order status response is invalid")
	}
	if response.Fulfillment != nil && validateReplicaFulfillment(response.Fulfillment) != nil {
		return errors.New("Website Replica fulfillment response is invalid")
	}
	if response.ServiceCase != nil {
		if response.Fulfillment == nil || validateReplicaServiceCase(response.ServiceCase) != nil || response.ServiceCase.OrderNo != response.OrderNo {
			return errors.New("Website Replica service case response is invalid")
		}
		if response.ServiceCase.FulfillmentID != response.Fulfillment.ID {
			return errors.New("Website Replica service case does not match fulfillment")
		}
	}
	return nil
}

func validateReplicaFulfillment(fulfillment *WebsiteReplicaFulfillment) error {
	if !zodUUIDPattern.MatchString(fulfillment.ID) || !validStringEnum(fulfillment.Status,
		"AWAITING_PAYMENT", "PENDING", "PROCESSING", "RECONCILING", "SUCCEEDED", "FAILED", "CANCELLED") ||
		!validPositiveSafeInteger(fulfillment.Version) || fulfillment.Tasks == nil {
		return errors.New("invalid fulfillment")
	}
	if fulfillment.CurrentTask != nil && validateReplicaFulfillmentTask(fulfillment.CurrentTask) != nil {
		return errors.New("invalid current fulfillment task")
	}
	for index := range fulfillment.Tasks {
		if validateReplicaFulfillmentTask(&fulfillment.Tasks[index]) != nil {
			return errors.New("invalid fulfillment task")
		}
	}
	return nil
}

func validateReplicaFulfillmentTask(task *WebsiteReplicaFulfillmentTask) error {
	if !zodUUIDPattern.MatchString(task.ID) || !validPositiveSafeInteger(task.Sequence) || !validPositiveSafeInteger(task.Version) ||
		!validStringEnum(task.CapabilityCode, "DIGITAL_ENTITLEMENT", "MANUAL_PROCESSING", "SHIPMENT", "PLATFORM_ADAPTER") ||
		!validStringEnum(task.Status, "BLOCKED", "READY", "PROCESSING", "RECONCILING", "SUCCEEDED", "FAILED", "CANCELLED") ||
		!validOptionalDatetime(task.StartedAt) || !validOptionalDatetime(task.CompletedAt) {
		return errors.New("invalid fulfillment task")
	}
	return nil
}

func validateReplicaServiceCase(serviceCase *WebsiteReplicaServiceCase) error {
	if !zodUUIDPattern.MatchString(serviceCase.ID) || utf16CodeUnits(serviceCase.CaseNo) < 8 || utf16CodeUnits(serviceCase.CaseNo) > 40 ||
		utf16CodeUnits(serviceCase.OrderNo) < 6 || utf16CodeUnits(serviceCase.OrderNo) > 40 || !zodUUIDPattern.MatchString(serviceCase.FulfillmentID) ||
		utf16CodeUnits(serviceCase.Work.CreatorHandle) < 2 || utf16CodeUnits(serviceCase.Work.CreatorHandle) > 32 ||
		utf16CodeUnits(serviceCase.Work.Slug) < 2 || utf16CodeUnits(serviceCase.Work.Slug) > 64 ||
		utf16CodeUnits(serviceCase.Work.Title) < 1 || utf16CodeUnits(serviceCase.Work.Title) > 200 ||
		!zodUUIDPattern.MatchString(serviceCase.Merchant.ID) || utf16CodeUnits(serviceCase.Merchant.DisplayName) < 1 ||
		!validServiceCaseStatus(serviceCase.Status) || utf16CodeUnits(serviceCase.CurrentStageCode) < 1 ||
		utf16CodeUnits(serviceCase.CurrentStageCode) > 80 || serviceCase.Stages == nil || serviceCase.Intake == nil ||
		serviceCase.PublicProgress == nil || !validPositiveSafeInteger(serviceCase.LockVersion) || serviceCase.Events == nil ||
		!validZodDatetime(serviceCase.SubmittedAt) || !validOptionalDatetime(serviceCase.CompletedAt) ||
		!validZodDatetime(serviceCase.UpdatedAt) {
		return errors.New("invalid service case")
	}
	for _, stage := range serviceCase.Stages {
		if utf16CodeUnits(stage.Code) < 1 || utf16CodeUnits(stage.Code) > 80 || utf16CodeUnits(stage.Label) < 1 || utf16CodeUnits(stage.Label) > 120 {
			return errors.New("invalid service case stage")
		}
	}
	for _, event := range serviceCase.Events {
		if !validPositiveSafeInteger(event.Sequence) || (event.FromStatus != nil && !validServiceCaseStatus(*event.FromStatus)) ||
			!validServiceCaseStatus(event.ToStatus) || utf16CodeUnits(event.StageCode) < 1 || utf16CodeUnits(event.StageCode) > 80 ||
			!validStringEnum(event.ActorType, "SYSTEM", "BUYER", "MERCHANT", "ADMIN") || !validZodDatetime(event.CreatedAt) {
			return errors.New("invalid service case event")
		}
	}
	return nil
}

func (response *WebsiteReplicaDownload) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.ReplicaID) || !zodUUIDPattern.MatchString(response.VersionID) ||
		!validPositiveSafeInteger(response.Version) || utf16CodeUnits(response.FileName) < 1 || utf16CodeUnits(response.FileName) > 255 ||
		response.SizeBytes < 1 || response.SizeBytes > maxJSONSafeInteger || !sha256HexPattern.MatchString(response.ArtifactDigest) ||
		!validAbsoluteURL(response.DownloadURL) || !validZodDatetime(response.ExpiresAt) || len(bytes.TrimSpace(response.License)) == 0 {
		return errors.New("Website Replica download response is invalid")
	}
	var license WebsiteReplicaLicense
	if err := decodeStrictAPIResponse(response.License, &license); err != nil || validateReplicaLicense(license) != nil {
		return errors.New("Website Replica license response is invalid")
	}
	if license.Claims.ReplicaID != response.ReplicaID || license.Claims.VersionID != response.VersionID || license.Claims.Version != response.Version ||
		license.Claims.ArtifactDigest != response.ArtifactDigest {
		return errors.New("Website Replica license does not match its download")
	}
	return nil
}

func (response *WebsiteReplicaInstallationReceipt) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.ReplicaID) ||
		!zodUUIDPattern.MatchString(response.VersionID) || !validPositiveSafeInteger(response.Version) ||
		!validZodDatetime(response.InstalledAt) {
		return errors.New("Website Replica installation receipt is invalid")
	}
	return nil
}

func validateReplicaLicense(license WebsiteReplicaLicense) error {
	claims := license.Claims
	if license.Algorithm != "Ed25519" || utf16CodeUnits(license.SigningKeyID) < 1 || utf16CodeUnits(license.SigningKeyID) > 64 ||
		utf16CodeUnits(license.SigningPublicKey) < 32 || utf16CodeUnits(license.Signature) < 32 ||
		claims.SchemaVersion != "website-replica-license/v1" || !zodUUIDPattern.MatchString(claims.EntitlementID) ||
		!zodUUIDPattern.MatchString(claims.ReplicaID) || !zodUUIDPattern.MatchString(claims.VersionID) ||
		!validPositiveSafeInteger(claims.Version) ||
		utf16CodeUnits(claims.OrderNo) < 6 || utf16CodeUnits(claims.OrderNo) > 40 || !sha256HexPattern.MatchString(claims.ArtifactDigest) ||
		utf16CodeUnits(claims.LicenseTermsVersion) < 1 || utf16CodeUnits(claims.LicenseTermsVersion) > 64 || !validZodDatetime(claims.IssuedAt) {
		return errors.New("invalid Website Replica license")
	}
	return nil
}

func validWebsiteReplicaPaymentAction(action *WebsiteReplicaPaymentAction) bool {
	return action != nil && action.Type == "QR_CODE" && action.Content != ""
}

func validReplicaPaymentOption(option WebsiteReplicaPaymentOption) bool {
	switch option.Provider {
	case "FREE", "BALANCE":
		return option.Scenes != nil && len(option.Scenes) == 0
	case "WECHAT_PAY":
		if len(option.Scenes) == 0 {
			return false
		}
		for _, scene := range option.Scenes {
			if !validStringEnum(scene, "NATIVE", "H5", "JSAPI") {
				return false
			}
		}
		return true
	case "ALIPAY":
		if len(option.Scenes) == 0 {
			return false
		}
		for _, scene := range option.Scenes {
			if !validStringEnum(scene, "PAGE", "WAP") {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func validZodDatetime(value string) bool {
	_, err := parseZodDatetime(value)
	return err == nil
}

func parseZodDatetime(value string) (time.Time, error) {
	match := zodDatetimePattern.FindStringSubmatch(value)
	if match == nil {
		return time.Time{}, errors.New("datetime does not match the API contract")
	}
	if match[2] == "" {
		value = match[1] + ":00Z"
	}
	return time.Parse(time.RFC3339Nano, value)
}

func validOptionalDatetime(value *string) bool {
	return value == nil || validZodDatetime(*value)
}

func validAbsoluteURL(value string) bool {
	parsed, err := shopURLParser.Parse(value)
	return err == nil && parsed.Scheme() != ""
}

func validOptionalUUID(value *string) bool {
	return value == nil || zodUUIDPattern.MatchString(*value)
}

func validNonnegativeSafeInteger(value int) bool {
	return value >= 0 && int64(value) <= maxJSONSafeInteger
}

func validPositiveSafeInteger(value int) bool {
	return value > 0 && int64(value) <= maxJSONSafeInteger
}

func validReplicaCurrency(value string) bool {
	return value == "CNY" || value == "USD"
}

func validServiceCaseStatus(value string) bool {
	return validStringEnum(value, "SUBMITTED", "ACCEPTED", "WAITING_BUYER", "IN_PROGRESS", "COMPLETED", "CANCELLED")
}

func validStringEnum(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
