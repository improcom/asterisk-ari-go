package main

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	asterisk_ari_go "github.com/olegromanchuk/asterisk-ari-go"
	"github.com/sirupsen/logrus"
	"log"
	"os"
	"sync"
	"time"
)

// handler is a struct that holds a logger.
type handler struct {
	Logger *logrus.Logger
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Error loading .env file")
	}

	ariHost := os.Getenv("ARI_HOST")
	ariUser := os.Getenv("ARI_USER")
	ariPass := os.Getenv("ARI_PASS")
	appName := os.Getenv("APP_NAME")
	if ariHost == "" || ariUser == "" || ariPass == "" {
		log.Fatal("ARI_HOST, ARI_USER and ARI_PASS environment variables must be set")
	}

	basicAuth := asterisk_ari_go.BasicAuth{
		UserName: ariUser,
		Password: ariPass,
	}
	authParam := ariUser + ":" + ariPass

	//set up logger
	logger := logrus.New()
	logLevel, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		logLevel = logrus.DebugLevel
		logger.Errorf("failed to parse log level: %v. Set to \"debug\"", err)
	}
	logger.SetLevel(logLevel)
	logger.SetOutput(os.Stdout)
	handler := handler{Logger: logger}

	// Initialize ARI client for REST communication
	conf := asterisk_ari_go.NewConfiguration("/")
	conf.Host = ariHost
	conf.Scheme = "ws"
	conf.UserAgent = "ARI_Client"

	logger.Infof("initializing ARI client app \"%s\" with next values: host: %s, user: %s, pass: %s", appName, ariHost, ariUser, "********")
	ariClient := asterisk_ari_go.NewAPIClient(conf, logger)

	//context to control and cancel goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure the context is canceled when main exits
	ctx = context.WithValue(ctx, asterisk_ari_go.ContextBasicAuth, basicAuth)

	var wg sync.WaitGroup
	wg.Add(1)

	go func(ctx context.Context) {
		defer wg.Done()
		//call function that will connect to Asterisk and start Stasis application
		handler.connectWebSocketAndStartStasis(ctx, authParam, ariClient, appName)
	}(ctx)
	logger.Info("started Asterisk ARI application goroutine")

	wg.Wait()
	logger.Info("\ntermination signal received. Shutting down gracefully.")
	logger.Info("shutdown complete.")
}

func (h handler) connectWebSocketAndStartStasis(ctx context.Context, authParam string, APIRESTClient *asterisk_ari_go.APIClient, appName string) {
	const (
		initialDelay = 1 * time.Second
		maxDelay     = 60 * time.Second
	)

	var wsConn *websocket.Conn
	var err error
	delay := initialDelay

	for {
		select {
		case <-ctx.Done():
			h.Logger.Debug("Context canceled, shutting down WebSocket connection.")
			return
		default:
			// Connect via WebSocket
			wsConn, _, err = APIRESTClient.WebsocketApi.WebsocketConnect(ctx, []string{appName}, []string{authParam})
			if err != nil {
				h.Logger.Errorf("Failed to connect via WebSocket: %v. Retrying in %v...", err, delay)
				// Use context-aware sleep
				select {
				case <-ctx.Done():
					h.Logger.Debug("Context canceled during backoff. Exiting.")
					return
				case <-time.After(delay):
				}
				delay = min(2*delay, maxDelay) // Exponential backoff
				continue
			}
			defer wsConn.Close()
			h.Logger.Debug("webSocket connection successfully established")

			// Reset delay on successful connection
			delay = initialDelay

			// Listen for incoming messages
		receiveLoop: //is needed to break to outer loop where socket reconnect is happening. If server restarts and socket closes, we need to reconnect.
			for {
				select {
				case <-ctx.Done():
					h.Logger.Debug("Context canceled, shutting down WebSocket connection.")
					defer wsConn.Close()
					return
				default:
					_, message, err := wsConn.ReadMessage()
					if err != nil {
						h.Logger.Errorf("Read error: %v. Attempting to reconnect...", err)
						break receiveLoop
					}

					var event asterisk_ari_go.StasisEvent
					err = json.Unmarshal(message, &event)
					if err != nil {
						h.Logger.Errorf("Error parsing JSON: %v", err)
					} else {
						formattedJSON, _ := json.MarshalIndent(event, "", "  ")
						h.Logger.Debugf("Received message: \n%s\n", string(formattedJSON))

						// Process StasisStart
						switch event.Type {
						case "StasisStart":
							h.Logger.Debugf("Received StasisStart message, app:%s, chType:%s\n", event.Application, event.Type)
						case "StasisEnd":
							h.Logger.Debugf("Received StasisEnd message, app:%s, chType:%s\n", event.Application, event.Type)
						default:
							h.Logger.Debugf("Unhandled event type: %s", event.Type)
						}
					}
				}
			}
		}

		// Wait before attempting to reconnect
		h.Logger.Infof("Reconnecting in %v...", delay)
		select {
		case <-ctx.Done():
			h.Logger.Debug("Context canceled during reconnect wait. Exiting.")
			return
		case <-time.After(delay):
		}
		delay = min(2*delay, maxDelay) // Exponential backoff
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
