package main

import (
	"context"
	"log"
	"time"
)

func RunCleanup(ctx context.Context, store *Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pitches, stamps, err := store.CleanExpired()
			if err != nil {
				log.Printf("cleanup error: %v", err)
				continue
			}
			if pitches > 0 || stamps > 0 {
				log.Printf("cleanup: removed %d expired pitches, %d used stamps", pitches, stamps)
			}
		}
	}
}
