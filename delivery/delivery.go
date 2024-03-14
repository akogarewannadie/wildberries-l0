package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

var (
	db *sql.DB
	nc *nats.Conn
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
	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}

	nc, err = nats.Connect("nats://localhost:4222")
	if err != nil {
		log.Fatalf("Unable to connect to NATS Streaming server: %v", err)
	}
}

func main() {
	defer db.Close()
	defer nc.Close()

	_, err := nc.Subscribe("order_request", handleOrderRequest)
	if err != nil {
		log.Fatalf("Error subscribing to order_request subject: %v", err)
	}

	select {}
}

func handleOrderRequest(msg *nats.Msg) {
	orderUID := string(msg.Data)

	log.Printf("Received message: %s", orderUID)

	orderData, err := getOrderFromDatabase(orderUID)
	if err != nil {
		log.Printf("Error getting order data from database: %v", err)
		return
	}

	if orderData == nil {
		log.Printf("No order data found for order UID: %s", orderUID)
		return
	}

	if err := sendOrderResponse(orderData, msg.Reply); err != nil {
		log.Printf("Error sending order data to client: %v", err)
	}
}

func getOrderFromDatabase(orderUID string) (*Order, error) {
	row := db.QueryRow("SELECT * FROM orders WHERE order_uid = $1", orderUID)

	var order Order
	err := row.Scan(&order.OrderUID, &order.TrackNumber, &order.Entry, &order.Delivery.Name, &order.Delivery.Phone, &order.Delivery.Zip, &order.Delivery.City, &order.Delivery.Address, &order.Delivery.Region, &order.Delivery.Email, &order.Payment.Transaction, &order.Payment.RequestID, &order.Payment.Currency, &order.Payment.Provider, &order.Payment.Amount, &order.Payment.PaymentDT, &order.Payment.Bank, &order.Payment.DeliveryCost, &order.Payment.GoodsTotal, &order.Payment.CustomFee, &order.Locale, &order.InternalSignature, &order.CustomerID, &order.DeliveryService, &order.ShardKey, &order.SMID, &order.DateCreated, &order.OofShard)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &order, nil

}

func sendOrderResponse(orderData *Order, replySubject string) error {

	log.Printf("Sending order response: %+v", orderData)

	orderJSON, err := json.Marshal(orderData)
	if err != nil {
		return err
	}

	err = nc.Publish("order_response", orderJSON)
	if err != nil {
		return err
	}

	return nil
}
