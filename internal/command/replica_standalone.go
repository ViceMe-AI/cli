package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
)

type standaloneReplicaAttempt struct {
	SchemaVersion  int    `json:"schemaVersion"`
	ReplicaID      string `json:"replicaId"`
	OrderNo        string `json:"orderNo"`
	RecoverySecret string `json:"recoverySecret"`
}

func standaloneReplicaAttemptMayExist(runtime *Runtime, shortCode string) bool {
	webURL, err := url.Parse(runtime.profile.ResolvedWebBaseURL())
	if err != nil || webURL.Scheme == "" || webURL.Host == "" {
		return false
	}
	fingerprint := sha256.Sum256([]byte(webURL.Scheme + "://" + webURL.Host + "/api/v1\n" + shortCode))
	filename := filepath.Join(runtime.configBase, "replica-purchases", "standalone-"+hex.EncodeToString(fingerprint[:])+".json")
	_, err = os.Lstat(filename)
	return err == nil
}

func loadStandaloneReplicaAttempt(runtime *Runtime, resolved api.WebsiteReplicaResolution) (standaloneReplicaAttempt, string, bool, error) {
	workURL, err := url.Parse(resolved.ViceMeWorkURL)
	if err != nil || workURL.Scheme != "https" || workURL.Host == "" {
		return standaloneReplicaAttempt{}, "", false, invalidReplicaResponse("Website Replica Work URL is invalid")
	}
	apiBaseURL := workURL.Scheme + "://" + workURL.Host + "/api/v1"
	fingerprint := sha256.Sum256([]byte(apiBaseURL + "\n" + resolved.ShortCode))
	filename := filepath.Join(runtime.configBase, "replica-purchases", "standalone-"+hex.EncodeToString(fingerprint[:])+".json")
	data, err := readReplicaBoundedFile(filename, 64<<10)
	if errors.Is(err, fs.ErrNotExist) {
		return standaloneReplicaAttempt{}, filename, false, nil
	}
	if err != nil {
		return standaloneReplicaAttempt{}, filename, false, output.Policy("REPLICA_STANDALONE_STATE_INVALID", "could not read the standalone Website Replica attempt").WithCause(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var attempt standaloneReplicaAttempt
	if err := decoder.Decode(&attempt); err != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		attempt.SchemaVersion != 1 || attempt.ReplicaID != resolved.ReplicaID ||
		len(attempt.OrderNo) < 6 || len(attempt.OrderNo) > 40 || !validReplicaSessionSecret(attempt.RecoverySecret) {
		return standaloneReplicaAttempt{}, filename, false, output.Policy("REPLICA_STANDALONE_STATE_INVALID", "standalone Website Replica attempt is invalid")
	}
	return attempt, filename, true, nil
}

func standaloneReplicaRecoveryAvailable(ctx context.Context, runtime *Runtime, resolved api.WebsiteReplicaResolution) (bool, error) {
	attempt, _, found, err := loadStandaloneReplicaAttempt(runtime, resolved)
	if err != nil || !found {
		return false, err
	}
	status, err := runtime.client().RecoverWebsiteReplicaOrderStatus(ctx, api.RecoverWebsiteReplicaDownloadRequest{
		OrderNo: attempt.OrderNo, RecoverySecret: attempt.RecoverySecret,
	})
	if err != nil {
		return false, err
	}
	return status.Payment.Status == "PAID", nil
}

func retireStandaloneUnpaidAttempt(ctx context.Context, runtime *Runtime, resolved api.WebsiteReplicaResolution) error {
	attempt, filename, found, err := loadStandaloneReplicaAttempt(runtime, resolved)
	if err != nil || !found {
		return err
	}
	client := runtime.client()
	status, err := client.RecoverWebsiteReplicaOrderStatus(ctx, api.RecoverWebsiteReplicaDownloadRequest{
		OrderNo: attempt.OrderNo, RecoverySecret: attempt.RecoverySecret,
	})
	if err != nil {
		return err
	}
	if status.Payment.Status == "PAID" {
		return output.Policy("REPLICA_STANDALONE_RECOVERY_REQUIRED", "a paid standalone Website Replica purchase must be recovered before creating another order")
	}
	if status.Payment.Status == "PENDING" {
		status, err = client.CancelWebsiteReplicaOrderAttempt(ctx, api.RecoverWebsiteReplicaDownloadRequest{
			OrderNo: attempt.OrderNo, RecoverySecret: attempt.RecoverySecret,
		})
		if err != nil {
			return err
		}
	}
	if status.Payment.Status != "CLOSED" {
		return invalidReplicaResponse("standalone Website Replica cancellation was not definitive")
	}
	if err := os.Remove(filename); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return output.Internal("REPLICA_STANDALONE_STATE_FAILED", "could not retire the standalone Website Replica attempt", err)
	}
	return nil
}
