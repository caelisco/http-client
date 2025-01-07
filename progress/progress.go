package progress

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80 // Default fallback width
	}
	return width
}

func CreateProgressFunc() func(int64, int64) {
	var lastUpdate time.Time
	var lastBytes int64

	return func(bytesRead, totalBytes int64) {
		now := time.Now()
		if now.Sub(lastUpdate) < 100*time.Millisecond {
			return
		}

		bytesSinceLast := bytesRead - lastBytes
		timeSinceLast := now.Sub(lastUpdate)
		var speed float64
		if timeSinceLast > 0 {
			speed = float64(bytesSinceLast) / timeSinceLast.Seconds()
		}

		width := getTerminalWidth() // Get terminal width dynamically

		// Define fixed widths for the progress bar and information
		const progressBarWidth = 50
		const infoWidth = 40

		// Handle progress display
		if totalBytes > 0 {
			percentage := float64(bytesRead) / float64(totalBytes) * 100
			if bytesRead >= totalBytes {
				percentage = 100 // Ensure it's exactly 100% when done
			}

			var eta time.Duration
			if speed > 0 && bytesRead < totalBytes {
				remainingBytes := totalBytes - bytesRead
				eta = time.Duration(float64(remainingBytes)/speed) * time.Second
			}

			var speedStr string
			switch {
			case speed >= 1024*1024*1024:
				speedStr = fmt.Sprintf("%.2f GB/s", speed/(1024*1024*1024))
			case speed >= 1024*1024:
				speedStr = fmt.Sprintf("%.2f MB/s", speed/(1024*1024))
			case speed >= 1024:
				speedStr = fmt.Sprintf("%.2f KB/s", speed/1024)
			default:
				speedStr = fmt.Sprintf("%.2f B/s", speed)
			}

			var etaStr string
			if eta > 0 {
				if eta >= time.Hour {
					etaStr = fmt.Sprintf("%.1fh", eta.Hours())
				} else if eta >= time.Minute {
					etaStr = fmt.Sprintf("%.1fm", eta.Minutes())
				} else {
					etaStr = fmt.Sprintf("%.0fs", eta.Seconds())
				}
			}

			// Create progress bar
			progressLength := int(float64(progressBarWidth) * (percentage / 100))
			bar := strings.Repeat("=", progressLength) + strings.Repeat(" ", progressBarWidth-progressLength)

			// Format the progress message
			var message string
			if percentage < 100 {
				message = fmt.Sprintf("\r[%s] %.2f%% | Speed: %s | ETA: %s", bar, percentage, speedStr, etaStr)
			} else {
				message = fmt.Sprintf("\r[%s] 100.00%% | Upload complete!", strings.Repeat("=", progressBarWidth))
			}

			// Ensure full message is displayed correctly
			paddedMessage := message
			padLength := width - len(message)
			if padLength > 0 {
				paddedMessage += strings.Repeat(" ", padLength)
			}

			// Print the message with padding
			fmt.Print(paddedMessage)

		} else {
			// Handle unknown total size
			message := fmt.Sprintf("\rUploaded %d bytes | Speed: %.2f MB/s", bytesRead, speed/(1024*1024))
			padLength := width - len(message)
			if padLength > 0 {
				message += strings.Repeat(" ", padLength)
			}
			fmt.Print(message)
		}

		lastUpdate = now
		lastBytes = bytesRead
	}
}
