package main

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time" // Не забудь этот импорт для таймаутов

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Message - структура должна совпадать с клиентской
type Message struct {
	SenderKey   string `json:"SenderKey"`
	ReceiverKey string `json:"ReceiverKey"` // <--- ДОБАВИЛИ ЭТО ПОЛЕ
	Content     []byte `json:"Content"`
	Nonce       []byte `json:"Nonce"`
	Signature   []byte `json:"Signature"`
}

type chatServer struct {
	subscriberMessageBuffer int
	publishLimiter          *sync.Mutex
	subscribers             map[*websocket.Conn]struct{}
}

func main() {
	cs := &chatServer{
		subscriberMessageBuffer: 16,
		publishLimiter:          &sync.Mutex{},
		subscribers:             make(map[*websocket.Conn]struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", cs.subscribeHandler)

	log.Println("Listening on :8443")
	err := http.ListenAndServe(":8443", mux)
	if err != nil {
		log.Fatal(err)
	}
}

func (cs *chatServer) subscribeHandler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("%v", err)
		return
	}
	defer c.Close(websocket.StatusInternalError, "")

	cs.addSubscriber(c)
	defer cs.deleteSubscriber(c)

	ctx := r.Context()

	for {
		var msg Message
		err := wsjson.Read(ctx, c, &msg)
		if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
			return
		}
		if err != nil {
			// Логируем ошибку, но выходим из цикла (клиент отключился или сбой сети)
			log.Printf("Client disconnected or read error: %v", err)
			return
		}

		// Лог для проверки, что ReceiverKey доходит
		log.Printf("Msg from %s to %s", shortKey(msg.SenderKey), shortKey(msg.ReceiverKey))

		cs.broadcast(ctx, msg)
	}
}

func (cs *chatServer) addSubscriber(c *websocket.Conn) {
	cs.publishLimiter.Lock()
	cs.subscribers[c] = struct{}{}
	cs.publishLimiter.Unlock()
	log.Println("New client connected")
}

func (cs *chatServer) deleteSubscriber(c *websocket.Conn) {
	cs.publishLimiter.Lock()
	delete(cs.subscribers, c)
	cs.publishLimiter.Unlock()
	log.Println("Client disconnected")
}

func (cs *chatServer) broadcast(ctx context.Context, msg Message) {
	cs.publishLimiter.Lock()
	defer cs.publishLimiter.Unlock()

	for c := range cs.subscribers {
		// Добавляем таймаут, чтобы один лагающий клиент не вешал всех
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)

		err := wsjson.Write(ctx, c, msg)
		if err != nil {
			log.Printf("failed to write to client: %v", err)
			// Здесь можно было бы удалять клиента, но deleteSubscriber сработает при ошибке чтения
		}
		cancel() // Обязательно вызываем cancel для очистки ресурсов таймера
	}
}

func shortKey(k string) string {
	if len(k) > 6 {
		return k[:6] + "..."
	}
	return k
}
