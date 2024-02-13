package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

var (
	secretKey = []byte("af0660f986d713761085f8ded052f25f")
	upgrader  = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	// Mapping from trader IDs to a list of subscribed WebSocket connections
	subscribedUsers        = make(map[string][]subscriber)
	subscribedUsersMutex   = sync.RWMutex{}
	natsSubscriptions      = make(map[string]*nats.Subscription)
	natsSubscriptionsMutex = sync.RWMutex{}
	userSubscriptions      = make(map[string]int)
	userSubscriptionsMutex = sync.RWMutex{}

	natsConn *nats.Conn
	rdb      *redis.Client
	ctx      = context.Background()
)

type subscriber struct {
	conn *websocket.Conn
	send chan []byte // Channel for sending messages
}

type Claims struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
	Subs int32  `json:"subs"`
	jwt.StandardClaims
}

type Message struct {
	Action  string `json:"action"`
	Channel string `json:"channel"`
}

// Struct for the request payload
type requestPayload struct {
	EncryptedUid string `json:"encryptedUid"`
}

// Struct for parsing the response
type apiResponse struct {
	Data *struct{} `json:"data"`
}

func initRedis() {
	// Read REDIS_PASSWORD from environment variables
	redisPassword := os.Getenv("REDIS_PASSWORD")
	log.Printf("REDIS_PASSWORD: %s", redisPassword)

	// Initialize Redis client
	rdb = redis.NewClient(&redis.Options{
		Addr:     "redis-server:6379", // or your Redis server address
		Password: redisPassword,       // set password from environment variable
		DB:       0,                   // use default DB
	})

	// Ensure the traders set is empty at start
	rdb.Del(ctx, "traders:um")
}

func initNATS() {
	// Read NATS_PASSWORD from environment variables
	natsPassword := os.Getenv("NATS_PASSWORD")
	log.Printf("NATS_PASSWORD: %s", natsPassword)
	natsURL := fmt.Sprintf("nats://admin:%s@nats-server:4222", natsPassword)

	var err error
	natsConn, err = nats.Connect(natsURL)
	if err != nil {
		log.Fatal("Failed to connect to NATS:", err)
	}
}

func initRedisTest() {
	// Initialize Redis client
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // or your Redis server address
		DB:   0,                // use default DB
	})

	// Ensure the traders set is empty at start
	rdb.Del(ctx, "traders:um")
}

func initNATSTest() {
	var err error
	natsConn, err = nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal("Failed to connect to NATS:", err)
	}
}

func authenticate(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return secretKey, nil
	})

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	} else {
		return nil, err
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	tokenString := r.Header.Get("Authorization")
	claims, err := authenticate(tokenString)
	if err != nil {
		log.Println("Authentication failed:", err)
		errorMsg := fmt.Sprintf("Authentication failed: %v", err)
		msg := websocket.FormatCloseMessage(websocket.ClosePolicyViolation, errorMsg)
		if writeErr := conn.WriteMessage(websocket.CloseMessage, msg); writeErr != nil {
			log.Println("Error sending close message:", writeErr)
		}
		return
	}

	// Initialize the send channel for this connection
	sendChannel := make(chan []byte, 256) // Adjust buffer size based on expected volume

	// Start the goroutine for managing writes to this connection
	go func(c *websocket.Conn, ch chan []byte) {
		for message := range ch {
			if err := c.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Error writing to websocket: %v", err)
				break // Exit the goroutine if we can't write to the connection
			}
		}
	}(conn, sendChannel)

	log.Printf("User connected: %s (%s)\n", claims.Name, claims.UID)
	defer func() {
		unsubscribeUserFromAll(conn, claims.Name, claims.UID)
		close(sendChannel)
		log.Printf("User disconnected: %s (%s)\n", claims.Name, claims.UID)
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Println("Failed to unmarshal message:", err)
			continue
		}

		traderID := msg.Channel[len("leader@"):]

		switch msg.Action {
		case "subscribe":
			if validateTrader(conn, traderID) {
				subscribeUserToTrader(conn, sendChannel, claims.Name, traderID, claims.UID, claims.Subs)
			}
		case "unsubscribe":
			unsubscribeUserFromTrader(conn, claims.Name, traderID, claims.UID)
		}
	}
}

func validateTraderRest(traderID string) bool {
	// The URL of the API endpoint
	url := "https://www.binance.com/bapi/futures/v2/public/future/leaderboard/getOtherLeaderboardBaseInfo"

	// Create the request payload
	payload := requestPayload{
		EncryptedUid: strings.ToUpper(traderID),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error encoding request payload: %v", err)
		return false
	}

	// Make the POST request
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("Error making POST request: %v", err)
		return false
	}
	defer resp.Body.Close()

	// Read and parse the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return false
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		log.Printf("Error parsing response JSON: %v", err)
		return false
	}

	// Check if the response contains non-nil data
	if apiResp.Data != nil {
		// Valid trader ID; add it to the Redis set "valid_traders:um"
		_, err := rdb.SAdd(ctx, "valid_traders:um", traderID).Result()
		if err != nil {
			log.Printf("Error adding trader ID to Redis: %v", err)
		}
		return true
	} else {
		// Invalid trader ID or other error
		return false
	}
}

func validateTrader(conn *websocket.Conn, traderID string) bool {
	// Check if the traderID exists in the Redis set 'valid_traders:um'
	exists, err := rdb.SIsMember(ctx, "valid_traders:um", traderID).Result()

	if err != nil {
		log.Printf("Error checking trader validity in Redis: %v", err)
		sendSubscribeResponse(conn, traderID, "subscribe", false, "Internal server error")
		return false
	}

	if !exists {
		// If the traderID does not exist in the set, check using validateTraderRest
		if validateTraderRest(traderID) {
			return true
		} else {
			// Trader ID is not valid according to validateTraderRest
			sendSubscribeResponse(conn, traderID, "subscribe", false, "Trader ID is invalid")
			return false
		}
	}

	// If we reach here, the traderID is valid (found in Redis set)
	return true
}

func sendSubscribeResponse(conn *websocket.Conn, traderID string, action string, success bool, error string) {
	data := map[string]interface{}{
		"action":  action,
		"channel": "leader@" + traderID,
		"success": success,
		"error":   error,
	}

	// Convert the map to a JSON string
	message, err := json.Marshal(data)
	if err != nil {
		log.Fatal("Error marshalling JSON: ", err)
	}

	// Send the JSON string as a websocket message
	err = conn.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		log.Fatal("Error sending message: ", err)
	}
}

func subscribeUserToTrader(conn *websocket.Conn, sendChannel chan []byte, userName string, traderID string, userID string, maxSubs int32) {
	subscribedUsersMutex.Lock()
	defer subscribedUsersMutex.Unlock()
	userSubscriptionsMutex.Lock()
	defer userSubscriptionsMutex.Unlock()

	// Check current subscription count for the user
	currentSubs, _ := userSubscriptions[userID]
	if currentSubs >= int(maxSubs) {
		log.Printf("User %s has reached the maximum subscription limit", userName)
		sendSubscribeResponse(conn, traderID, "subscribe", false, "Maximum subscription limit reached")
		return
	}

	// Ensure the traderID is lowercased for consistency
	traderID = strings.ToLower(traderID)
	subscribers, ok := subscribedUsers[traderID]
	if !ok {
		subscribers = []subscriber{}
		// New trader ID added, so update Redis and NATS
		updateSubscriptionsInRedisAndNATS(traderID)
	}

	// Check if the user is already subscribed to avoid duplicate subscriptions
	alreadySubscribed := false
	for _, sub := range subscribers {
		if sub.conn == conn {
			alreadySubscribed = true
			break
		}
	}

	if !alreadySubscribed {
		userSubscriptions[userID] = currentSubs + 1

		// Append the new subscriber with the connection and send channel
		subscribers = append(subscribers, subscriber{conn, sendChannel})
		subscribedUsers[traderID] = subscribers

		sendSubscribeResponse(conn, traderID, "subscribe", true, "")
		log.Printf("User %s subscribed to %s", userName, traderID)
	} else {
		log.Printf("User %s is already subscribed to %s", userName, traderID)
		sendSubscribeResponse(conn, traderID, "subscribe", false, "Duplicated subscription")
	}
}

func unsubscribeUserFromTrader(conn *websocket.Conn, userName string, traderID string, userID string) {
	subscribedUsersMutex.Lock()
	defer subscribedUsersMutex.Unlock()
	userSubscriptionsMutex.Lock()
	defer userSubscriptionsMutex.Unlock()

	traderID = strings.ToLower(traderID)
	subscribers, ok := subscribedUsers[traderID]
	if !ok {
		return
	}

	for i, subscriber := range subscribers {
		if subscriber.conn == conn {
			subscribers = append(subscribers[:i], subscribers[i+1:]...)
			// Decrease the user's subscription count
			currentSubs, _ := userSubscriptions[userID]
			userSubscriptions[userID] = currentSubs - 1
			break
		}
	}

	if len(subscribers) == 0 {
		delete(subscribedUsers, traderID)
	} else {
		subscribedUsers[traderID] = subscribers
	}

	sendSubscribeResponse(conn, traderID, "unsubscribe", true, "")

	log.Printf("User %s unsubscribed from %s", userName, traderID)
}

func unsubscribeUserFromAll(conn *websocket.Conn, userName string, userID string) {
	subscribedUsersMutex.Lock()
	defer subscribedUsersMutex.Unlock()
	userSubscriptionsMutex.Lock()
	defer userSubscriptionsMutex.Unlock()

	for traderID, subscribers := range subscribedUsers {
		for i, subscriber := range subscribers {
			if subscriber.conn == conn {
				subscribers = append(subscribers[:i], subscribers[i+1:]...)
				log.Printf("User %s unsubscribed from %s", userName, traderID)

				// Decrement user's subscription count for each unsubscribe
				currentSubs, _ := userSubscriptions[userID]
				if currentSubs > 0 {
					userSubscriptions[userID] = currentSubs - 1
				}

				break // Assuming a connection cannot subscribe multiple times to the same trader
			}
		}

		if len(subscribers) == 0 {
			delete(subscribedUsers, traderID)
			removeFromRedisAndNATS(traderID)
		} else {
			subscribedUsers[traderID] = subscribers
		}
	}
}

func updateSubscriptionsInRedisAndNATS(traderID string) {
	traderID = strings.ToLower(traderID)

	// Update Redis
	_, err := rdb.SAdd(ctx, "traders:um", traderID).Result()
	if err != nil {
		log.Printf("Error updating traders in Redis: %v", err)
	}

	// Subscribe to NATS channel
	channelName := fmt.Sprintf("trader:um:%s", traderID)
	sub, err := natsConn.Subscribe(channelName, func(msg *nats.Msg) {
		sendUpdates(traderID, msg.Data)
	})
	if err != nil {
		log.Printf("Failed to subscribe to NATS channel for trader %s: %v", traderID, err)
	} else {
		natsSubscriptionsMutex.Lock()
		natsSubscriptions[traderID] = sub
		natsSubscriptionsMutex.Unlock()
	}
}

func removeFromRedisAndNATS(traderID string) {
	traderID = strings.ToLower(traderID)

	// Remove from Redis
	_, err := rdb.SRem(ctx, "traders:um", traderID).Result()
	if err != nil {
		log.Printf("Error removing trader from Redis: %v", err)
	}

	// Unsubscribe from NATS
	natsSubscriptionsMutex.Lock()
	sub, ok := natsSubscriptions[traderID]
	if ok {
		if err := sub.Unsubscribe(); err != nil {
			log.Printf("Error unsubscribing from NATS for trader %s: %v", traderID, err)
		}
		delete(natsSubscriptions, traderID)
	}
	natsSubscriptionsMutex.Unlock()
}

func sendUpdates(traderID string, message []byte) {
	subscribedUsersMutex.RLock()
	subscribers, ok := subscribedUsers[traderID]
	subscribedUsersMutex.RUnlock()

	if ok {
		for _, sub := range subscribers {
			sub.send <- message // Send the message to the subscriber's send channel
		}
	}
}

func main() {
	initRedis()
	initNATS()

	http.HandleFunc("/ws", handler)
	log.Println("WebSocket server started on :8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

//func main() {
//	initRedis()
//	initNATS()
//
//	http.HandleFunc("/ws", handler)
//	log.Println("WebSocket Secure server started on :443")
//
//	// Specify the paths to your certificate and private key files
//	certPath := "/etc/letsencrypt/live/leaderboardapi.net/fullchain.pem"
//	keyPath := "/etc/letsencrypt/live/leaderboardapi.net/privkey.pem"
//
//	// Use ListenAndServeTLS instead of ListenAndServe
//	if err := http.ListenAndServeTLS(":443", certPath, keyPath, nil); err != nil {
//		log.Fatal("ListenAndServeTLS: ", err)
//	}
//}

//func main() {
//	initRedisTest()
//	initNATSTest()
//
//	http.HandleFunc("/ws", handler)
//	log.Println("WebSocket Secure server started on :443")
//
//	log.Println("Starting HTTP server on port 80")
//	if err := http.ListenAndServe(":80", nil); err != nil {
//		log.Fatal("ListenAndServe: ", err)
//	}
//}
