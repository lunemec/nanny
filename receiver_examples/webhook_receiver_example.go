package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
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
			http.Error(w, "Method Not Allowed", 405)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			http.Error(w, "Unsupported Media Type", 415)
			return
		}

		sendingProgram := r.Header.Get("X-Program")
		log.Println("X-Program:            " + sendingProgram)

		currentTimestamp := strconv.FormatInt(time.Now().Unix(), 10)
		requestTimestamp := r.Header.Get("X-Timestamp")
		hmacSHA256 := r.Header.Get("X-HMAC-SHA256")

		body, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			http.Error(w, "Internal Server Error", 500)
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
				http.Error(w, "Unauthorized", 401)
				return
			}
			currentTimestampInt, err := strconv.Atoi(currentTimestamp)
			if err != nil {
				http.Error(w, "Internal Server Error", 500)
				return
			}
			requestTimestampInt, err := strconv.Atoi(requestTimestamp)
			if err != nil {
				http.Error(w, "Bad Request", 400)
				return
			}
			if int(math.Abs(float64(currentTimestampInt-requestTimestampInt))) > 10 {
				log.Println("Timestamp is older than 10 seconds")
				http.Error(w, "Bad Request", 400)
				return
			} else {
				log.Println("Timestamp is not older than 10 seconds")
			}
		}
		log.Println(string(body))
		log.Println("--------------------------------------------------------------------------------------")
	})

	http.ListenAndServe(":8081", nil)
}
