package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// incomingChannel is for messages we get from the server
// outgoingChannel is for messages we send to the server
func NewWebsocket(endpoint, accessToken string) (*WebSocket, error) {

	subscriptions := make(map[string]*WebsocketSubscription)
	socket := &WebSocket{
		subscriptions:      subscriptions,
		endpoint:           endpoint,
		accessToken:        accessToken,
		dataMutex:          &sync.Mutex{},
		sendMutex:          &sync.Mutex{},
		subscriptionsMutex: &sync.Mutex{},
	}

	err := socket.listen()

	if err != nil {
		return nil, err
	}

	return socket, nil
}

func (socket *WebSocket) getConnection() (*websocket.Conn, error) {

	url, err := url.Parse(socket.endpoint)

	if err != nil {
		return nil, err
	}

	protocol := "ws"

	if url.Scheme == "https" {
		protocol = "wss"
	}

	url.Scheme = protocol
	url.Path = "/api/v1/websocket/connect"

	urlToPrint := url.String()
	query := url.Query()
	query.Set("access_token", socket.accessToken)
	url.RawQuery = query.Encode()

	if os.Getenv("DEBUG_WEBSOCKETS") != "" {
		log.Printf("[websocket] connecting to %s", urlToPrint)
	}

	connection, _, err := websocket.DefaultDialer.Dial(url.String(), nil)
	if err != nil {
		return nil, err
	}

	return connection, nil
}

func (socket *WebSocket) reconnect() {
	for {
		err := socket.listen()
		if err != nil {
			log.Printf("[websocket] websocket reconnect error: %v", err)
			time.Sleep(time.Second * 1)
		} else {
			break
		}
	}
}

func (socket *WebSocket) listen() error {
	connection, err := socket.getConnection()

	if err != nil {
		return err
	}

	socket.dataMutex.Lock()
	socket.connection = connection
	err = socket.SubscribeExisting()
	socket.dataMutex.Unlock()
	if err != nil {
		return err
	}

	go func() {
		for {
			_, envelopeBytes, err := connection.ReadMessage()
			if err != nil {
				log.Printf("websocket connection error: %v", err)
				break
			}
			envelope := &WebsocketEnvelope{}
			err = json.Unmarshal(envelopeBytes, &envelope)
			if os.Getenv("DEBUG_WEBSOCKETS") != "" {
				log.Printf("[websocket] got message: %s", string(envelopeBytes))
			}
			// we couldn't parse the JSON but don't disconnect
			if err != nil {
				log.Printf("websocket json parse error: '%s' - %s", string(envelopeBytes), err.Error())
				continue
			}

			// reply to ping messages right away
			if envelope.Handler == "ping" {
				socket.SendText("pong", "", "", "")
			} else {
				if envelope.Channel != "" {
					socket.subscriptionsMutex.Lock()
					subscription, ok := socket.subscriptions[envelope.Channel]
					socket.subscriptionsMutex.Unlock()

					if ok {
						select {
						case subscription.IncomingChannel <- *envelope:
							// succeeded to send synchronously.
						default:
							// channel full. send to it later (risks
							// re-ordering messages, but probably better than
							// throwing them on the floor.)
							go func() {
								subscription.IncomingChannel <- *envelope
							}()
						}
					}
				}
			}
		}

		socket.dataMutex.Lock()
		socket.connection = nil
		socket.dataMutex.Unlock()
		connection.Close()
		time.Sleep(time.Second * 1)
		socket.reconnect()
	}()

	return nil
}

func (socket *WebSocket) SubscribeExisting() error {
	socket.subscriptionsMutex.Lock()
	subscriptions := socket.subscriptions
	socket.subscriptionsMutex.Unlock()
	for channel := range subscriptions {
		if os.Getenv("DEBUG_WEBSOCKETS") != "" {
			log.Printf("[websocket] subscribing to existing channel: %s", channel)
		}
		err := socket.SendText("subscribe", channel, "", "")
		if err != nil {
			return err
		}
	}
	return nil
}

func (socket *WebSocket) SendText(handler, channel, messageType, body string) error {
	socket.sendMutex.Lock()
	defer socket.sendMutex.Unlock()
	if socket.connection == nil {
		return fmt.Errorf("[websocket] Websocket has no active connection")
	}
	envelopeBytes, err := json.Marshal(WebsocketEnvelope{
		Handler:     handler,
		Channel:     channel,
		MessageType: messageType,
		Body:        body,
	})
	if err != nil {
		return err
	}
	if os.Getenv("DEBUG_WEBSOCKETS") != "" {
		log.Printf("[websocket] send message handler: %s", string(envelopeBytes))
	}
	return socket.connection.WriteMessage(websocket.TextMessage, envelopeBytes)
}

func (socket *WebSocket) SendJSON(handler, channel, messageType string, body interface{}) error {
	bytes, err := json.Marshal(body)
	if err != nil {
		return err
	}
	socket.SendText(handler, channel, messageType, string(bytes))
	return nil
}

// subscribe to a channel
func (socket *WebSocket) Subscribe(channel string) (*WebsocketSubscription, error) {
	socket.dataMutex.Lock()
	defer socket.dataMutex.Unlock()
	err := socket.SendText("subscribe", channel, "", "")
	if err != nil {
		return nil, err
	}
	subscription := &WebsocketSubscription{
		Channel:         channel,
		IncomingChannel: make(chan WebsocketEnvelope, 1024),
	}
	socket.subscriptionsMutex.Lock()
	socket.subscriptions[channel] = subscription
	socket.subscriptionsMutex.Unlock()
	return subscription, nil
}

func (socket *WebSocket) Unsubscribe(channel string) error {
	socket.dataMutex.Lock()
	defer socket.dataMutex.Unlock()
	err := socket.SendText("unsubscribe", channel, "", "")
	if err != nil {
		return err
	}
	socket.subscriptionsMutex.Lock()
	defer socket.subscriptionsMutex.Unlock()
	delete(socket.subscriptions, channel)
	return nil
}
