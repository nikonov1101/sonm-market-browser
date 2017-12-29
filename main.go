package main

import (
	"crypto"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	ds "github.com/c2h5oh/datasize"
	ethc "github.com/ethereum/go-ethereum/crypto"
	"github.com/labstack/echo"
	"github.com/pkg/errors"
	"github.com/sonm-io/core/insonmnia/structs"
	pb "github.com/sonm-io/core/proto"
	"github.com/sonm-io/core/util"
	"github.com/sonm-io/core/util/xgrpc"
	"golang.org/x/net/context"
)

// SONM MVP marketplace
var marketAddr = "0xa99faeef5559b823b63eb8fe51cc13e14907c909@188.166.190.188:9095"

// my local testing instance
// var marketAddr = "0x733193d40B6F03c3da33Dbb2e0e070aCbBf8d91b@127.0.0.1:9095"

type App struct {
	ctx    context.Context
	key    crypto.PrivateKey
	market pb.MarketClient
}

func newApp(ctx context.Context) (*App, error) {
	key, err := ethc.GenerateKey()
	if err != nil {
		return nil, err
	}

	_, TLSConfig, err := util.NewHitlessCertRotator(ctx, key)
	if err != nil {
		return nil, err
	}

	remoteCreds := util.NewTLS(TLSConfig)
	cc, err := xgrpc.NewWalletAuthenticatedClient(ctx, remoteCreds, marketAddr)
	if err != nil {
		return nil, err
	}

	return &App{
		ctx:    ctx,
		key:    key,
		market: pb.NewMarketClient(cc),
	}, nil
}

// TemplateRenderer is a custom html/template renderer for Echo framework
type TemplateRenderer struct {
	templates *template.Template
}

// Render renders a template document
func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {

	// Add global methods if data is a map
	if viewContext, isMap := data.(map[string]interface{}); isMap {
		viewContext["reverse"] = c.Echo().Reverse
	}

	return t.templates.ExecuteTemplate(w, name, data)
}

func main() {
	ctx := context.Background()

	log.Println("Starting remotes...")
	app, err := newApp(ctx)
	if err != nil {
		log.Printf("Cannot create app: %v\r\n", err)
		os.Exit(1)
	}

	e := echo.New()
	e.Renderer = &TemplateRenderer{
		templates: template.Must(template.ParseGlob("./static/*.html")),
	}

	e.GET("/", func(c echo.Context) error {
		log.Println("handling index request")
		return c.Render(http.StatusOK, "index.html", nil)
	})

	e.GET("/search/", func(c echo.Context) error {
		log.Println("handling search request")
		order, err := reqContextToOrder(c)
		if err != nil {
			log.Printf("cannot convert request to order: %v\r\n", err)
			return c.String(http.StatusBadRequest, err.Error())
		}

		count, err := strconv.ParseUint(c.QueryParam("count"), 10, 64)
		if err != nil {
			count = 25
		}

		marketReq := &pb.GetOrdersRequest{
			Order: order,
			Count: count,
		}

		orders, err := app.market.GetOrders(ctx, marketReq)
		if err != nil {
			log.Printf("cannot retrieve orders from Maretplace: %v\r\n", err)
			return c.String(http.StatusBadRequest, err.Error())
		}

		data := make([]*row, 0, len(orders.GetOrders()))
		for _, item := range orders.GetOrders() {
			data = append(data, orderToRow(item))
		}

		return c.JSON(http.StatusOK, map[string]interface{}{"data": data})
	})

	log.Println("Starting web server...")
	err = e.Start(":8087")
	if err != nil {
		log.Printf("cannot start the Echo http server: %v\r\n", err)
		os.Exit(1)
	}
}

func reqContextToOrder(c echo.Context) (*pb.Order, error) {
	clientID := c.QueryParam("client_id")
	pricePerSec := c.QueryParam("pps")
	price, err := pb.NewBigIntFromString(pricePerSec)
	if err != nil {
		return nil, err
	}

	orderType, err := strconv.ParseInt(c.QueryParam("type"), 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert order type")
	}

	duration, err := strconv.ParseInt(c.QueryParam("duration"), 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert order duration")
	}

	cpu, err := strconv.ParseUint(c.QueryParam("cpu"), 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert order cpu")
	}

	gpu, err := strconv.ParseInt(c.QueryParam("gpu"), 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert order gpu")
	}

	ram, err := strconv.ParseUint(c.QueryParam("ram"), 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert order ram")
	}

	storage, err := strconv.ParseUint(c.QueryParam("storage"), 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert order storage")
	}

	netType, err := strconv.ParseInt(c.QueryParam("net_type"), 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert order net_type")
	}

	netIn, err := strconv.ParseUint(c.QueryParam("net_in"), 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert order net_in")
	}

	netOut, err := strconv.ParseUint(c.QueryParam("net_out"), 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert order net_out")
	}

	order := &pb.Order{
		// price field is temporary hacked
		PricePerSecond: pb.NewBigIntFromInt(1),
		Price:          price.Unwrap().String(),
		OrderType:      pb.OrderType(orderType),
		Slot: &pb.Slot{
			Duration: uint64(duration),
			Resources: &pb.Resources{
				CpuCores:      cpu,
				RamBytes:      ram,
				GpuCount:      pb.GPUCount(gpu),
				Storage:       storage,
				NetworkType:   pb.NetworkType(netType),
				NetTrafficIn:  netIn,
				NetTrafficOut: netOut,
			},
		},
	}

	if order.OrderType == pb.OrderType_ASK {
		order.SupplierID = clientID
	} else {
		order.ByuerID = clientID
	}

	_, err = structs.NewOrder(order)
	if err != nil {
		return nil, errors.Wrap(err, "order is malformed")
	}

	return order, nil
}

// row represents table row with search results
type row struct {
	ID        string `json:"id"`
	ClientID  string `json:"client_id"`
	OrderType string `json:"order_type"`
	Price     string `json:"price"`
	Duration  string `json:"duration"`
	CPU       string `json:"cpu"`
	GPU       string `json:"gpu"`
	RAM       string `json:"ram"`
	NetType   string `json:"net_type"`
	NetIn     string `json:"net_in"`
	NetOut    string `json:"net_out"`
}

// orderToRow converts found order to row representation
func orderToRow(o *pb.Order) *row {
	r := &row{
		ID:        o.GetId(),
		OrderType: o.OrderType.String(),
		// Price:     o.PricePerSecond.Unwrap().String(),
		Price:    o.GetPrice(),
		Duration: time.Duration(time.Duration(o.GetSlot().GetDuration()) * time.Second).String(),
		CPU:      fmt.Sprintf("%d", o.GetSlot().GetResources().GetCpuCores()),
		GPU:      o.GetSlot().GetResources().GetGpuCount().String(),
		NetType:  o.GetSlot().GetResources().GetNetworkType().String(),
		RAM:      ds.ByteSize(o.GetSlot().GetResources().GetRamBytes()).HR(),
		NetIn:    ds.ByteSize(o.GetSlot().GetResources().GetNetTrafficIn()).HR(),
		NetOut:   ds.ByteSize(o.GetSlot().GetResources().GetNetTrafficOut()).HR(),
	}

	if o.GetOrderType() == pb.OrderType_ASK {
		r.ClientID = o.GetSupplierID()
	} else {
		r.ClientID = o.GetByuerID()
	}

	return r
}
