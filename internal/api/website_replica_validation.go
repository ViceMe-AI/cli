package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxJSONSafeInteger = int64(1<<53 - 1)

var (
	zodDatetimePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2})(?::(\d{2})(?:\.(\d+))?)?Z$`)
	zodUUIDPattern     = regexp.MustCompile(`(?i)^(?:[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}|00000000-0000-0000-0000-000000000000|ffffffff-ffff-ffff-ffff-ffffffffffff)$`)
	jsonRawMessageType = reflect.TypeOf(json.RawMessage{})
	replicaActionType  = reflect.TypeOf((*WebsiteReplicaPaymentAction)(nil))
)

type strictAPIResponse interface {
	strictAPIResponse()
}

func (*CreateWebsiteReplicaUploadResponse) strictAPIResponse()   {}
func (*CompleteWebsiteReplicaUploadResponse) strictAPIResponse() {}
func (*WebsiteReplicaResolution) strictAPIResponse()             {}
func (*WebsiteReplicaQuote) strictAPIResponse()                  {}
func (*WebsiteReplicaOrder) strictAPIResponse()                  {}
func (*WebsiteReplicaOrderStatus) strictAPIResponse()            {}
func (*WebsiteReplicaDownload) strictAPIResponse()               {}

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
			if !found {
				return fmt.Errorf("%s.%s is required", path, name)
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
	case "REDIRECT":
		var value struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		}
		if err := decodeStrictAPIResponse(data, &value); err != nil {
			return err
		}
		*action = WebsiteReplicaPaymentAction{Type: value.Type, URL: value.URL}
	case "JSAPI":
		var value struct {
			Type      string `json:"type"`
			AppID     string `json:"appId"`
			TimeStamp string `json:"timeStamp"`
			NonceStr  string `json:"nonceStr"`
			Package   string `json:"package"`
			SignType  string `json:"signType"`
			PaySign   string `json:"paySign"`
		}
		if err := decodeStrictAPIResponse(data, &value); err != nil {
			return err
		}
		*action = WebsiteReplicaPaymentAction{
			Type: value.Type, AppID: value.AppID, TimeStamp: value.TimeStamp,
			NonceStr: value.NonceStr, Package: value.Package, SignType: value.SignType, PaySign: value.PaySign,
		}
	default:
		return errors.New("Website Replica payment action type is invalid")
	}
	return nil
}

func (response *CreateWebsiteReplicaUploadResponse) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.ReplicaID) || !zodUUIDPattern.MatchString(response.UploadID) ||
		response.Upload.Method != "PUT" || !validAbsoluteURL(response.Upload.URL) || response.Upload.Headers == nil ||
		!validZodDatetime(response.Upload.ExpiresAt) {
		return errors.New("Website Replica upload response is invalid")
	}
	return nil
}

func (response *CompleteWebsiteReplicaUploadResponse) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.ReplicaID) || !zodUUIDPattern.MatchString(response.VersionID) ||
		!validPositiveSafeInteger(response.Version) || !websiteReplicaCodePattern.MatchString(response.ShortCode) ||
		response.Instruction != "VICEME-REPLICA:"+response.ShortCode || validateWebsiteReplicaProduct(response.Product) != nil ||
		!validZodDatetime(response.PublishedAt) {
		return errors.New("Website Replica publication response is invalid")
	}
	return nil
}

func (response *WebsiteReplicaResolution) validateAPIResponse() error {
	if response == nil || !zodUUIDPattern.MatchString(response.ReplicaID) || !websiteReplicaCodePattern.MatchString(response.ShortCode) ||
		utf16CodeUnits(response.Title) < 1 || utf16CodeUnits(response.Title) > 200 || utf16CodeUnits(response.Summary) > 500 ||
		utf16CodeUnits(response.Creator.Handle) < 2 || utf16CodeUnits(response.Creator.Handle) > 32 ||
		validateWebsiteReplicaProduct(response.Product) != nil {
		return errors.New("Website Replica resolution response is invalid")
	}
	return nil
}

func validateWebsiteReplicaProduct(product WebsiteReplicaProduct) error {
	if !zodUUIDPattern.MatchString(product.ID) || !zodUUIDPattern.MatchString(product.SKUID) || utf16CodeUnits(product.Title) < 1 ||
		utf16CodeUnits(product.Title) > 200 || !validReplicaCurrency(product.Currency) || !validPositiveSafeInteger(product.PriceCents) {
		return errors.New("Website Replica product is invalid")
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
		response.PaymentOptions[0].Provider == "WECHAT_PAY" &&
		len(response.PaymentOptions[0].Scenes) == 1 &&
		response.PaymentOptions[0].Scenes[0] == "NATIVE"
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
	if license.Claims.ReplicaID != response.ReplicaID || license.Claims.VersionID != response.VersionID ||
		license.Claims.ArtifactDigest != response.ArtifactDigest {
		return errors.New("Website Replica license does not match its download")
	}
	return nil
}

func validateReplicaLicense(license WebsiteReplicaLicense) error {
	claims := license.Claims
	if license.Algorithm != "Ed25519" || utf16CodeUnits(license.SigningKeyID) < 1 || utf16CodeUnits(license.SigningKeyID) > 64 ||
		utf16CodeUnits(license.SigningPublicKey) < 32 || utf16CodeUnits(license.Signature) < 32 ||
		claims.SchemaVersion != "website-replica-license/v1" || !zodUUIDPattern.MatchString(claims.EntitlementID) ||
		!zodUUIDPattern.MatchString(claims.ReplicaID) || !zodUUIDPattern.MatchString(claims.VersionID) ||
		utf16CodeUnits(claims.OrderNo) < 6 || utf16CodeUnits(claims.OrderNo) > 40 || !sha256HexPattern.MatchString(claims.ArtifactDigest) ||
		utf16CodeUnits(claims.LicenseTermsVersion) < 1 || utf16CodeUnits(claims.LicenseTermsVersion) > 64 || !validZodDatetime(claims.IssuedAt) {
		return errors.New("invalid Website Replica license")
	}
	return nil
}

func validWebsiteReplicaPaymentAction(action *WebsiteReplicaPaymentAction) bool {
	if action == nil {
		return false
	}
	switch action.Type {
	case "QR_CODE":
		return action.Content != ""
	case "REDIRECT":
		return validAbsoluteURL(action.URL)
	default:
		return false
	}
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
	match := zodDatetimePattern.FindStringSubmatch(value)
	if match == nil {
		return false
	}
	if _, err := time.Parse("2006-01-02T15:04", match[1]); err != nil {
		return false
	}
	if match[2] == "" {
		return true
	}
	seconds, err := strconv.Atoi(match[2])
	return err == nil && seconds >= 0 && seconds <= 59
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
