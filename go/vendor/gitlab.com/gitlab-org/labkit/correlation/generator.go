package correlation

import (
	"crypto/rand"
	"fmt"
	"log"
	"math"
	"math/big"
	"net/http"
	"time"
)

var (
	randMax    = big.NewInt(math.MaxInt64)
	randSource = rand.Reader
)

// generateRandomCorrelationID will attempt to generate a correlationid randomly
// or raise an error
func generateRandomCorrelationID() (string, error) {
	id, err := rand.Int(randSource, randMax)
	if err != nil {
		return "", err
	}
	base62 := encodeReverseBase62(id.Int64())

	return base62, nil
}

func generatePseudorandomCorrelationID(req *http.Request) string {
	return fmt.Sprintf("E:%s:%s", req.RemoteAddr, encodeReverseBase62(time.Now().UnixNano()))
}

// generateRandomCorrelationID will attempt to generate a correlationid randomly
// if this fails, will log a message and fallback to a pseudorandom approach
func generateRandomCorrelationIDWithFallback(req *http.Request) string {
	correlationID, err := generateRandomCorrelationID()
	if err == nil {
		return correlationID
	}

	log.Printf("can't generate random correlation-id: %v", err)
	return generatePseudorandomCorrelationID(req)
}
