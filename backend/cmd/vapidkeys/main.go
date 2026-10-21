// Command vapidkeys prints a new VAPID key pair for Web Push. Put both values
// in the backend environment only; the frontend fetches the public key from
// GET /api/push/public-key.
package main

import (
	"fmt"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("VAPID_PUBLIC_KEY=%s\nVAPID_PRIVATE_KEY=%s\nVAPID_SUBJECT=mailto:you@example.com\n", pub, priv)
}
