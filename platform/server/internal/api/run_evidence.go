package api

import (
	"bytes"
	"encoding/base64"
	"image/jpeg"
)

const (
	maxRunScreenshots      = 4
	maxStepScreenshotBytes = 128 * 1024
	maxScreenshotWidth     = 1280
	maxScreenshotHeight    = 720
)

type runStep struct {
	Number          int    `json:"step"`
	Action          string `json:"action"`
	Status          string `json:"status"`
	DurationMS      int    `json:"duration_ms"`
	ScreenshotJPEG  string `json:"screenshot_jpeg,omitempty"`
	ScreenshotError string `json:"screenshot_error,omitempty"`
}

func validRunSteps(steps []runStep) bool {
	screenshots := 0
	for index, step := range steps {
		if step.Number != index+1 || step.Action == "" || step.DurationMS < 0 || (step.Status != "passed" && step.Status != "failed") {
			return false
		}
		if step.ScreenshotError != "" {
			if step.ScreenshotJPEG != "" {
				return false
			}
			switch step.ScreenshotError {
			case "unavailable", "too_large", "limit":
			default:
				return false
			}
		}
		if step.ScreenshotJPEG != "" {
			screenshots++
			if screenshots > maxRunScreenshots || !validStepScreenshot(step.ScreenshotJPEG) {
				return false
			}
		}
	}
	return true
}

func validStepScreenshot(encoded string) bool {
	if len(encoded) > base64.StdEncoding.EncodedLen(maxStepScreenshotBytes) {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) > maxStepScreenshotBytes {
		return false
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	return err == nil && config.Width > 0 && config.Width <= maxScreenshotWidth && config.Height > 0 && config.Height <= maxScreenshotHeight
}
