//go:build !darwin && !linux

package messages

import "time"

func probeKittyGraphics(time.Duration) bool { return false }
