package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"
)

func ComputeHmacSha256(secret string, message []byte) string {
	key := []byte(secret)
	h := hmac.New(sha256.New, key)
	h.Write(message)
	return hex.EncodeToString(h.Sum(nil))
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		webhookSecret := ""

		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
			return
		}

		sendingProgram := r.Header.Get("X-Program")
		log.Println("X-Program:            " + sendingProgram)

		currentTimestamp := strconv.FormatInt(time.Now().Unix(), 10)
		requestTimestamp := r.Header.Get("X-Timestamp")
		hmacSHA256 := r.Header.Get("X-HMAC-SHA256")

		body, err := io.ReadAll(r.Body)
		if closeErr := r.Body.Close(); closeErr != nil {
			log.Printf("close request body: %v", closeErr)
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if requestTimestamp != "" && hmacSHA256 != "" {
			payload := append([]byte(requestTimestamp), body...)
			signature := ComputeHmacSha256(webhookSecret, payload)
			log.Println("X-Timestamp:          " + requestTimestamp)
			log.Println("Current timestamp:    " + currentTimestamp)
			log.Println("X-HMAC-SHA256:        " + hmacSHA256)
			log.Println("Calculated signature: " + signature)
			if hmacSHA256 == signature {
				log.Println("Signature is correct")
			} else {
				log.Println("Signature is not correct")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			currentTimestampInt, err := strconv.Atoi(currentTimestamp)
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			requestTimestampInt, err := strconv.Atoi(requestTimestamp)
			if err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			if int(math.Abs(float64(currentTimestampInt-requestTimestampInt))) > 10 {
				log.Println("Timestamp is older than 10 seconds")
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			} else {
				log.Println("Timestamp is not older than 10 seconds")
			}
		}
		log.Println(string(body))
		log.Println("--------------------------------------------------------------------------------------")
	})

	log.Fatal(http.ListenAndServe(":8081", nil))
}
