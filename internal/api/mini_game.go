package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const MiniGameRuntimeVersion = "1.0.0"

var miniGameAliasPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var miniGameClientPattern = regexp.MustCompile(`^vca_[A-Za-z0-9_-]{32}$`)
var miniGameUUIDShapedAlias = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type MiniGameItem struct {
	ID     string `json:"id"`
	Alias  string `json:"alias"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type MiniGameIntegration struct {
	SchemaVersion  int            `json:"schemaVersion"`
	RuntimeVersion string         `json:"runtimeVersion"`
	WorkID         string         `json:"workId"`
	WorkTitle      string         `json:"workTitle"`
	Environment    string         `json:"environment"`
	PublicClientID string         `json:"publicClientId"`
	PublicKey      string         `json:"publicKey"`
	CheckoutOrigin string         `json:"checkoutOrigin"`
	Items          []MiniGameItem `json:"items"`
}

func (*MiniGameIntegration) strictAPIResponse() {}

func (m *MiniGameIntegration) validateAPIResponse() error {
	if m == nil || m.SchemaVersion != 1 || m.RuntimeVersion != MiniGameRuntimeVersion || !uuidPattern.MatchString(m.WorkID) || m.WorkID != strings.ToLower(m.WorkID) || !miniGameText(m.WorkTitle) || !validCommerceApplicationEnvironment(m.Environment) || !miniGameClientPattern.MatchString(m.PublicClientID) || m.Items == nil {
		return errors.New("invalid mini-game integration identity or version")
	}
	key, err := base64.RawURLEncoding.Strict().DecodeString(m.PublicKey)
	if err != nil || len(key) != 32 || len(m.PublicKey) != 43 {
		return errors.New("invalid mini-game public key")
	}
	if !validCommerceApplicationOrigin(m.CheckoutOrigin) {
		return errors.New("invalid mini-game checkout origin")
	}
	ids, aliases := map[string]bool{}, map[string]bool{}
	for _, item := range m.Items {
		id := strings.ToLower(item.ID)
		if !uuidPattern.MatchString(item.ID) || item.ID != id || len(item.Alias) < 2 || len(item.Alias) > 120 || !miniGameAliasPattern.MatchString(item.Alias) || miniGameUUIDShapedAlias.MatchString(item.Alias) || !miniGameText(item.Title) || !validProductStatus(item.Status) || ids[id] || aliases[item.Alias] {
			return errors.New("invalid or duplicate mini-game item identity, alias or status")
		}
		ids[id], aliases[item.Alias] = true, true
	}
	return nil
}

func miniGameText(value string) bool {
	return strings.TrimSpace(value) == value && value != "" && len(value) <= 2000 && !strings.ContainsAny(value, "\x00\r\n")
}

func (c *Client) GetMiniGameIntegration(ctx context.Context, workID, merchantAccountID, environment string) (MiniGameIntegration, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	query := url.Values{"merchantAccountId": {merchantAccountID}, "environment": {environment}}
	var result MiniGameIntegration
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/works/"+url.PathEscape(workID)+"/mini-game-integration?"+query.Encode(), nil, &result, "@stored")
	if err == nil && (!strings.EqualFold(result.WorkID, workID) || result.Environment != environment) {
		err = invalidAPIResponse(errors.New("mini-game response belongs to another Work or environment"))
	}
	return result, err
}
