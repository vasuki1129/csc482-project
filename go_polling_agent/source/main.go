package main

//AWS_ACCESS_KEY_ID
//AWS_SECRET_ACCESS_KEY

import (
	"context"
	"encoding/json"
	"io"
	"fmt"
	"log"
	"strconv"
	"unsafe" //how DARE google tell me what is and isn't safe for me and my family
	"net/http"
	"time"
	"slices"
	"cmp"
	"os"
	"strings"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)


var loggly_url = os.Getenv("LOGGLY_URL")

var additional_polling_wait = time.Duration(0) * time.Minute




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

type DBOrder struct {
	IsBuyOrder  bool `dynamodbav:"IsBuyOrder"`
	LocationId  int  `dynamodbav:"LocationId"`
	Price       float32 `dynamodbav:"Price"`
	TypeId      int     `dynamodbav:"TypeId"`
	VolumeRemain int    `dynamodbav:"VolumeRemain"`
	VolumeTotal  int    `dynamodbav:"VolumeTotal"`

}


type DBRecord struct {
	Timestamp int64 `dynamodbav:"Timestamp"`
	TypeId int `dynamodbav:"TypeId"`
	OrderBook []DBOrder `dynamodbav:"OrderBook"`
}


type LogLevel int

const (
	Info LogLevel = iota
	Warn
	Error
)


func connect_db() *dynamodb.Client {
	ctx := context.Background()
	
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}
	

	client := dynamodb.NewFromConfig(cfg)
	
	return client
}

func save_record(data []Order, client *dynamodb.Client) {
	var orders []DBOrder

	lTID := -1
	for _,value := range data {
		lTID = value.TypeId
		
		orders = append(orders, DBOrder {
			IsBuyOrder: value.IsBuyOrder,
			LocationId: value.LocationId,
			Price : value.Price,
			TypeId : value.TypeId,
			VolumeRemain: value.VolumeRemain,
			VolumeTotal: value.VolumeTotal})
	}


	var record DBRecord
	
	record.OrderBook = orders
	record.TypeId = lTID
	record.Timestamp = time.Now().Unix()

	av,err := attributevalue.MarshalMap(record)
	if err != nil {
		log.Fatalf("Failed to marshal order: %v", err)
	}

	tableName := "mmorabi2_EveOrderTable"
	_, err = client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: av})
	if err != nil {
		log.Fatalf("failed to put item into DynamoDB: %v", err)
	}
	fmt.Println("Successfully put data to DynamoDB")
}








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



func poll_data(client *dynamodb.Client) int64  {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://esi.evetech.net/markets/10000002/orders?type_id=30013", nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
		log_loggly("Failed to create polling request", Error)
	}

	http_client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := http_client.Do(req)
	if err != nil {
		log.Fatalf("Request execution failed: %v", err)
		log_loggly("Polling request execution failed.", Error)
	}
	defer resp.Body.Close() 

	if resp.StatusCode != http.StatusOK {
		log_loggly("Server returned unexpected status " + resp.Status, Error)
		log.Fatalf("Server returned unexpected status: %s", resp.Status)
	}

	expires_header := resp.Header.Get("Expires")
	t1, err := time.Parse(time.RFC1123, expires_header)
	currentTime := time.Now()
	distance := int64(t1.Sub(currentTime).Seconds())+5

	bytes_read, err := io.ReadAll(resp.Body)

	var data []Order
	if err := json.Unmarshal(bytes_read, &data); err != nil {
		log_loggly("Failed to unmarshal response data", Error)
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

	buy_orders = buy_orders[0:]
	sell_orders = sell_orders[0:]

	for _, val := range buy_orders {
		fmt.Printf("%+v\n", val)
	}

	for _, val := range sell_orders {
		fmt.Printf("%+v\n", val)


	}

	orders_list := slices.Concat(buy_orders,sell_orders)
	
	save_record(orders_list,client)
	
        log_loggly("Successfully polled data, saved " + strconv.Itoa(len(orders_list)) + " orders, " + strconv.Itoa(len(orders_list) * int(unsafe.Sizeof(orders_list[0]))) + " bytes of data.", Info)
        return distance
}

func main() {

	client := connect_db()
	var nextPoll int64 = 0
	for
	{
		timer := time.NewTimer(time.Duration(nextPoll) * time.Second + additional_polling_wait)
		_ = <-timer.C
		nextPoll = poll_data(client)
		fmt.Printf("Polling again in %d seconds\n", nextPoll)
	}
}
