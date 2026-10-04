package main

import (
	"context"
	"encoding/json"
	"io"
	"fmt"
	"log"
	"net/http"
	"time"
	"slices"
	"cmp"
	"strings"
)

var loggly_url = "http://logs-01.loggly.com/inputs/095e0c6f-57d2-48fd-a348-bc1109deada9/tag/http/"

var additional_polling_wait = time.Duration(0) * time.Minute

var order_book_depth = 5

type Order struct {
//	Duration   int    `json:"duration"`
	IsBuyOrder bool `json:"is_buy_order"`
//	Issued     string `json:"issued"`
	LocationId int    `json:"location_id"`
//	MinVolume  int    `json:"min_volume"`
//	OrderId    int    `json:"order_id"`
	Price      float32  `json:"price"`
//	Range      string `json:"range"`
//	SystemId   int    `json:"system_id"`
	TypeId     int    `json:"type_id"`
	VolumeRemain int  `json:"volume_remain"`
	VolumeTotal int   `json:"volume_total"`
}

type LogLevel int

const (
	Info LogLevel = iota
	Warn
	Error
)

func log_loggly(str string, log_level LogLevel){

body := strings.NewReader(str)

req, err := http.NewRequest("POST", loggly_url, body)
if err != nil {
	panic("Could not create logging request")
}

if log_level == Info {
	req.Header.Set("X-LOGGLY-TAG","Info")
} else if log_level == Warn {
	req.Header.Set("X-LOGGLY-TAG","Warning")
} else if log_level == Error {
	req.Header.Set("X-LOGGLY-TAG","Error")
} else {
	fmt.Printf("Incorrect logging level supplied to log_loggly()\n")
	return
}

client := &http.Client {
		Timeout: 10 * time.Second,
	}

	resp,err := client.Do(req)
	if err != nil {
		log.Fatalf("Error, could not send log data to Loggly")
	}
	defer resp.Body.Close()
}

func poll_data() int64  {

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://esi.evetech.net/markets/10000002/orders?type_id=30013", nil)
if err != nil {
	log.Fatalf("Failed to create request: %v", err)
}

client := &http.Client{
	Timeout: 5 * time.Second,
}

resp, err := client.Do(req)
if err != nil {
	log.Fatalf("Request execution failed: %v", err)
}
defer resp.Body.Close() 

if resp.StatusCode != http.StatusOK {
	log.Fatalf("Server returned unexpected status: %s", resp.Status)
}

expires_header := resp.Header.Get("Expires")
t1, err := time.Parse(time.RFC1123, expires_header)
currentTime := time.Now()
distance := int64(t1.Sub(currentTime).Seconds())+5

bytes_read, err := io.ReadAll(resp.Body)

var data []Order
if err := json.Unmarshal(bytes_read, &data); err != nil {
 panic(err)
}

var buy_orders []Order
var sell_orders []Order

for _, item := range data {
	if item.LocationId == 60003760 {	
		if item.IsBuyOrder {
			buy_orders = append(buy_orders, item)
		} else {
			sell_orders = append(sell_orders, item)
		}
	}
}

slices.SortFunc(buy_orders, func(a, b Order) int {
	return cmp.Compare(b.Price,a.Price)
})

slices.SortFunc(sell_orders, func(a, b Order) int {
	return cmp.Compare(a.Price,b.Price)
})

buy_orders = buy_orders[0:order_book_depth]
sell_orders = sell_orders[0:order_book_depth]

for _, val := range buy_orders {
	fmt.Printf("%+v\n", val)
}

for _, val := range sell_orders {
	fmt.Printf("%+v\n", val)
}


log_loggly("Successfully polled data", Info)

return distance
}

func main() {
	var nextPoll int64 = 0
	for
	{
		timer := time.NewTimer(time.Duration(nextPoll) * time.Second + additional_polling_wait)
		_ = <-timer.C
		nextPoll = poll_data()
		fmt.Printf("Polling again in %d seconds\n", nextPoll)
	}
}
