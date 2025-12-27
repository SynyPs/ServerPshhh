package main

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Message - структура, совпадающая с тем, что шлет клиент
// Серверу не обязательно знать содержимое, но для логов полезно
type Message struct {
	SenderKey string `json:"SenderKey"`
	Content   []byte `json:"Content"` // Зашифрованные байты
	Nonce     []byte `json:"Nonce"`
	Signature []byte `json:"Signature"`
}

type chatServer struct {
	// subscriberMessageBuffer - размер буфера канала для каждого клиента
	subscriberMessageBuffer int

	// mutex защищает карту подписчиков
	publishLimiter *sync.Mutex

	// subscribers - активные соединения
	subscribers map[*websocket.Conn]struct{}
}

func main() {
	cs := &chatServer{
		subscriberMessageBuffer: 16,
		publishLimiter:          &sync.Mutex{},
		subscribers:             make(map[*websocket.Conn]struct{}),
	}

	// Настраиваем HTTP роутер
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", cs.subscribeHandler)

	log.Println("Listening on :8443")
	// Запускаем сервер
	err := http.ListenAndServe(":8443", mux)
	if err != nil {
		log.Fatal(err)
	}
}

// subscribeHandler обрабатывает входящее подключение
func (cs *chatServer) subscribeHandler(w http.ResponseWriter, r *http.Request) {
	// Принимаем WebSocket соединение
	// InsecureSkipVerify: true нужен для тестов, чтобы не ругался на CORS/Origin
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

	// Читаем сообщения в бесконечном цикле
	for {
		var msg Message
		// Читаем JSON в структуру
		err := wsjson.Read(ctx, c, &msg)
		if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
			return
		}
		if err != nil {
			log.Printf("failed to read message: %v", err)
			return
		}

		log.Printf("Received msg from %s (len: %d)", shortKey(msg.SenderKey), len(msg.Content))

		// Рассылаем всем остальным
		cs.broadcast(ctx, msg)
	}
}

// addSubscriber добавляет клиента в список
func (cs *chatServer) addSubscriber(c *websocket.Conn) {
	cs.publishLimiter.Lock()
	cs.subscribers[c] = struct{}{}
	cs.publishLimiter.Unlock()
	log.Println("New client connected")
}

// deleteSubscriber удаляет клиента
func (cs *chatServer) deleteSubscriber(c *websocket.Conn) {
	cs.publishLimiter.Lock()
	delete(cs.subscribers, c)
	cs.publishLimiter.Unlock()
	log.Println("Client disconnected")
}

// broadcast отправляет сообщение всем подключенным клиентам
func (cs *chatServer) broadcast(ctx context.Context, msg Message) {
	cs.publishLimiter.Lock()
	defer cs.publishLimiter.Unlock()

	for c := range cs.subscribers {
		// ВАЖНО: Мы убрали "go func", теперь отправка идет последовательно.
		// Это предотвращает гонку данных при записи в один и тот же сокет.
		// Для хайлоада тут нужна более сложная структура (очередь на каждого клиента),
		// но для чата это идеальное решение, чтобы не падал сервер.

		err := wsjson.Write(ctx, c, msg)
		if err != nil {
			log.Printf("failed to write to client: %v", err)
			// Если ошибка записи - можно удалять клиента, но пока оставим так
		}
	}
}

// Вспомогательная функция для красивых логов
func shortKey(k string) string {
	if len(k) > 6 {
		return k[:6] + "..."
	}
	return k
}
