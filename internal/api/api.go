package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cleverdata/sift-agent/internal/config"
	"github.com/go-resty/resty/v2"
)

func Pinger(ctx context.Context, remote config.RemoteConfig, logger func(string, ...interface{})) {
	client := resty.New()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			resp, err := client.R().
				SetHeader("Authorization", "Bearer "+remote.Key).
				Get(remote.Endpoint + "/agent/check")

			if err != nil {
				if logger != nil {
					logger("[%s] Heartbeat failed: %v", remote.Name, err)
				}
			} else if resp.StatusCode() != 200 {
				if logger != nil {
					logger("[%s] Heartbeat rejected: Status %d", remote.Name, resp.StatusCode())
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func UploadFile(ctx context.Context, remote config.RemoteConfig, filePath string, modTime int64,
	onSuccess func(string, string, int64), onError func(string), logger func(string, ...interface{})) {

	client := resty.New()

	f, err := os.Open(filePath)
	if err != nil {
		return
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		f.Close()
		return
	}
	localHash := hex.EncodeToString(hasher.Sum(nil))
	f.Close()

	for i := 0; i < 3; i++ {
		resp, err := client.R().
			SetContext(ctx).
			SetHeader("Authorization", "Bearer "+remote.Key).
			SetFile("file", filePath).
			Post(fmt.Sprintf("%s/agent/upload", remote.Endpoint))

		if err == nil && resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
			if onSuccess != nil {
				onSuccess(filePath, localHash, modTime)
			}
			return
		}

		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return
		}
	}
	if onError != nil {
		onError(filePath)
	}
}
