// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"dcrpulse/internal/alerts"
	"dcrpulse/internal/config"
)

// StartDiskAlerts probes the dashboard's writable mounts at startup and then
// hourly, raising disk_high per mount at 90% (warning) and escalating to
// critical at 95%.
func StartDiskAlerts(ctx context.Context) {
	go func() {
		probeDiskAlerts()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				probeDiskAlerts()
			}
		}
	}()
}

func probeDiskAlerts() {
	for _, mount := range []string{config.AppDataDir, config.AppDataRoot} {
		var st syscall.Statfs_t
		if err := syscall.Statfs(mount, &st); err != nil || st.Blocks == 0 {
			continue
		}
		used := 100 - float64(st.Bavail)/float64(st.Blocks)*100
		detail := fmt.Sprintf("%s is %.0f%% full", mount, used)
		switch {
		case used >= 95:
			alerts.RaiseSeverity("disk_high", mount, detail, alerts.SeverityCritical)
		case used >= 90:
			alerts.Raise("disk_high", mount, detail)
		default:
			alerts.Resolve("disk_high", mount)
		}
	}
}
