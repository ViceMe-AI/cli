package command

import "time"

const (
	replicaTestProductID = "66666666-6666-4666-8666-666666666666"
	replicaTestSKUID     = "77777777-7777-4777-8777-777777777777"
	replicaTestWorkID    = "88888888-8888-4888-8888-888888888888"
)

func replicaResolutionResponse(replicaID, shortCode string) map[string]any {
	return map[string]any{
		"replicaId": replicaID,
		"shortCode": shortCode,
		"title":     "Replica",
		"summary":   "Immutable website source",
		"creator": map[string]any{
			"handle":      "replica-maker",
			"displayName": "Replica Maker",
		},
		"viceMeWorkUrl": "https://viceme.example/replica-maker/replica",
		"availability":  "AVAILABLE",
		"product":       replicaProductResponse(),
	}
}

func replicaQuoteResponse(quoteID string) map[string]any {
	return map[string]any{
		"id": quoteID,
		"product": map[string]any{
			"id": replicaTestProductID, "slug": "replica-source", "title": "Replica",
		},
		"attribution": map[string]any{
			"subjectWorkId": replicaTestWorkID, "entryWorkId": nil, "commerceApplicationId": nil,
		},
		"sku": map[string]any{
			"id": replicaTestSKUID, "code": "default", "title": "\u6c38\u4e45\u6e90\u7801\u4e0b\u8f7d", "selectedOptions": map[string]string{},
		},
		"currency":            "CNY",
		"unitAmountCents":     990,
		"quantity":            1,
		"subtotalAmountCents": 990,
		"shippingAmountCents": 0,
		"totalAmountCents":    990,
		"contractSummary":     map[string]any{"publicFields": map[string]any{}, "sensitiveFieldKeys": []string{}, "assetCount": 0},
		"fulfillment":         map[string]any{"capabilities": []string{"DIGITAL_ENTITLEMENT"}, "estimatedState": "AWAITING_PAYMENT"},
		"paymentOptions":      []map[string]any{{"provider": "WECHAT_PAY", "scenes": []string{"NATIVE"}}},
		"expiresAt":           time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	}
}

func replicaPublicationResponse(replicaID, versionID, shortCode string) map[string]any {
	instruction := "VICEME-REPLICA:" + shortCode
	return map[string]any{
		"replicaId":   replicaID,
		"versionId":   versionID,
		"version":     1,
		"shortCode":   shortCode,
		"instruction": instruction,
		"product":     replicaProductResponse(),
		"buyerEntry": map[string]any{
			"instruction": instruction,
			"prompts": map[string]any{
				"zh-CN": "先展示真实 Quote，明确确认后再追加 --confirm。",
				"en-US": "Show the authoritative quote, then append --confirm only after explicit confirmation.",
			},
			"viceMeWorkUrl": "https://viceme.example/replica-maker/replica-source",
		},
		"publishedAt": time.Now().UTC().Format(time.RFC3339),
	}
}

func replicaProductResponse() map[string]any {
	return map[string]any{
		"id": replicaTestProductID, "skuId": replicaTestSKUID,
		"title": "Replica", "currency": "CNY", "priceCents": 990,
	}
}
