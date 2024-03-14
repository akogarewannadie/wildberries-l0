package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

var (
	nc    *nats.Conn
	cache = make(map[string]Order)
	mu    sync.Mutex
)

type Order struct {
	OrderUID          string    `json:"order_uid"`
	TrackNumber       string    `json:"track_number"`
	Entry             string    `json:"entry"`
	Delivery          Delivery  `json:"delivery"`
	Payment           Payment   `json:"payment"`
	Items             []Item    `json:"items"`
	Locale            string    `json:"locale"`
	InternalSignature string    `json:"internal_signature"`
	CustomerID        string    `json:"customer_id"`
	DeliveryService   string    `json:"delivery_service"`
	ShardKey          string    `json:"shardkey"`
	SMID              int       `json:"sm_id"`
	DateCreated       time.Time `json:"date_created"`
	OofShard          string    `json:"oof_shard"`
}

type Delivery struct {
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Zip     string `json:"zip"`
	City    string `json:"city"`
	Address string `json:"address"`
	Region  string `json:"region"`
	Email   string `json:"email"`
}

type Payment struct {
	Transaction  string  `json:"transaction"`
	RequestID    string  `json:"request_id"`
	Currency     string  `json:"currency"`
	Provider     string  `json:"provider"`
	Amount       float64 `json:"amount"`
	PaymentDT    int64   `json:"payment_dt"`
	Bank         string  `json:"bank"`
	DeliveryCost float64 `json:"delivery_cost"`
	GoodsTotal   float64 `json:"goods_total"`
	CustomFee    float64 `json:"custom_fee"`
}

type Item struct {
	ChrtID      int     `json:"chrt_id"`
	TrackNumber string  `json:"track_number"`
	Price       float64 `json:"price"`
	RID         string  `json:"rid"`
	Name        string  `json:"name"`
	Sale        float64 `json:"sale"`
	Size        string  `json:"size"`
	TotalPrice  float64 `json:"total_price"`
	NMID        int     `json:"nm_id"`
	Brand       string  `json:"brand"`
	Status      int     `json:"status"`
}
type Response struct {
	Success bool   `json:"success"`
	Order   *Order `json:"order"`
}

func init() {
	var err error
	nc, err = nats.Connect("nats://localhost:4222")
	if err != nil {
		log.Fatalf("Unable to connect to NATS Streaming server: %v", err)
	}
}

func main() {
	defer nc.Close()

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/order/", orderHandler)

	go func() {
		nc.Subscribe("order_request", func(msg *nats.Msg) {
			var orderReq Order
			if err := json.Unmarshal(msg.Data, &orderReq); err != nil {
				log.Printf("Error decoding order request: %v", err)
				return
			}

			mu.Lock()
			order, ok := cache[orderReq.OrderUID]
			mu.Unlock()

			if ok {
				sendOrderResponse(order, msg.Reply)
				return
			}

			if err := nc.Publish("order_query", msg.Data); err != nil {
				log.Printf("Error sending order query: %v", err)
			}
		})

		nc.Subscribe("order_response", func(msg *nats.Msg) {
			var orderResp Order
			if err := json.Unmarshal(msg.Data, &orderResp); err != nil {
				log.Printf("Error decoding order response: %v", err)
				return
			}

			mu.Lock()
			cache[orderResp.OrderUID] = orderResp
			mu.Unlock()
		})
	}()

	log.Fatal(http.ListenAndServe(":8081", nil))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func orderHandler(w http.ResponseWriter, r *http.Request) {
	orderUID := r.URL.Query().Get("orderUID")

	if orderUID == "" {
		log.Println("Empty order UID")
		http.Error(w, "Empty order UID", http.StatusBadRequest)
		return
	}

	order, err := getOrder(orderUID)
	if err != nil {
		log.Printf("Error getting order: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := Response{
		Success: true,
		Order:   order,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func getOrder(orderUID string) (*Order, error) {

	mu.Lock()
	order, ok := cache[orderUID]
	mu.Unlock()

	if ok {
		return &order, nil
	}

	sub, err := nc.SubscribeSync("order_response")
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe()

	if err := nc.Publish("order_request", []byte(orderUID)); err != nil {
		return nil, err
	}

	msg, err := sub.NextMsg(500 * time.Millisecond)
	if err != nil {
		return nil, err
	}

	var orderResp Order
	if err := json.Unmarshal(msg.Data, &orderResp); err != nil {
		return nil, err
	}

	return &orderResp, nil
}

func sendOrderResponse(order Order, replySubject string) {

	log.Printf("Sending order response: %+v", order)

	orderJSON, err := json.Marshal(order)
	if err != nil {
		log.Printf("Error encoding order response: %v", err)
		return
	}

	if err := nc.Publish(replySubject, orderJSON); err != nil {
		log.Printf("Error sending order response: %v", err)
	}
}
