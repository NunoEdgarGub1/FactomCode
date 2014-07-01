package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"github.com/NotaryChains/NotaryChainCode/notaryapi"
	"github.com/conformal/btcrpcclient"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/firelizzard18/dynrsrc"
	"github.com/firelizzard18/gobundle"
	"github.com/firelizzard18/gocoding"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"log"
)

var client *btcrpcclient.Client
var currentAddr *btcutil.Address
var balance int64

var portNumber = flag.Int("p", 8083, "Set the port to listen on")
var blocks []*notaryapi.Block
var blockMutex = &sync.Mutex{}
var tickers [2]*time.Ticker

func watchError(err error) {
	panic(err)
}

func readError(err error) {
	fmt.Println("error: ", err)
}

func initWithBinary() {
	matches, err := filepath.Glob(gobundle.DataFile("store.*.block"))
	if err != nil {
		panic(err)
	}

	blocks = make([]*notaryapi.Block, len(matches))

	num := 0
	for _, match := range matches {
		data, err := ioutil.ReadFile(match)
		if err != nil {
			panic(err)
		}

		block := new(notaryapi.Block)
		err = block.UnmarshalBinary(data)
		if err != nil {
			panic(err)
		}

		blocks[num] = block
		num++
	}
}

func init() {
	gobundle.Setup.Application.Name = "NotaryChains/restapi"
	gobundle.Init()

	dynrsrc.Start(watchError, readError)
	notaryapi.StartDynamic(gobundle.DataFile("html.gwp"), readError)

	initWithBinary()

	fmt.Println("Loaded", len(blocks), "blocks")

	for i := 0; i < len(blocks); i = i + 1 {
		if uint64(i) != blocks[i].BlockID {
			panic(errors.New("BlockID does not equal index"))
		}
	}
	notaryapi.UpdateNextBlockID(uint64(len(blocks)))

	tickers[0] = time.NewTicker(time.Minute * 5)
	tickers[1] = time.NewTicker(time.Hour)

	go func() {
		for _ = range tickers[1].C {
			save()
		}
	}()
}



func main() {

	cadr, err := btcutil.DecodeAddress("mtmkwxRDToguqs6q9dX526wq8xxh8TCbGB", activeNet.Params)

	currentAddr = &cadr

	// Only override the handlers for notifications you care about.
	// Also note most of the handlers will only be called if you register
	// for notifications.  See the documentation of the btcrpcclient
	// NotificationHandlers type for more details about each handler.
	ntfnHandlers := btcrpcclient.NotificationHandlers{
		OnAccountBalance: func(account string, balance btcutil.Amount, confirmed bool) {
		     go newBalance(account, balance, confirmed)
	    },

		OnBlockConnected: func(hash *btcwire.ShaHash, height int32) {
			go newBlock(hash, height)
		},
	}
	
	// Connect to local btcwallet RPC server using websockets.
	certHomeDir := btcutil.AppDataDir("btcwallet", false)
	certs, err := ioutil.ReadFile(filepath.Join(certHomeDir, "rpc.cert"))
	if err != nil {
		log.Fatal(err)
	}
	connCfg := &btcrpcclient.ConnConfig{
		Host:         "localhost:18332",
		Endpoint:     "ws",
		User:         "testuser",
		Pass:         "notarychain",
		Certificates: certs,
	}
	
	client, err = btcrpcclient.New(connCfg, &ntfnHandlers)
	
	if err != nil {
		log.Fatal(err)
		return;
	}else{
        defer shutdown(client) 
    }  		
	
	
	flag.Parse()

	defer func() {
		tickers[0].Stop()
		tickers[1].Stop()
		save()
		dynrsrc.Stop()
	}()

	http.HandleFunc("/", serveRESTfulHTTP)
	err = http.ListenAndServe(":"+strconv.Itoa(*portNumber), nil)
	if err != nil {
		panic(err)
	}
}

func save() {
	bcp := make([]*notaryapi.Block, len(blocks))

	blockMutex.Lock()
	copy(bcp, blocks)
	blockMutex.Unlock()

	for i, block := range bcp {
		data, err := block.MarshalBinary()
		if err != nil {
			panic(err)
		}

		err = ioutil.WriteFile(fmt.Sprintf(`app/rest/store.%d.block`, i), data, 0777)
		if err != nil {
			panic(err)
		}
	}
}

func serveRESTfulHTTP(w http.ResponseWriter, r *http.Request) {
	var resource interface{}
	var err *notaryapi.Error
	var buf bytes.Buffer

	path, method, accept, form, err := parse(r)

	defer func() {
		switch accept {
		case "text":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		case "json":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")

		case "xml":
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")

		case "html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}

		if err != nil {
			var r *notaryapi.Error

			buf.Reset()
			r = notaryapi.Marshal(err, accept, &buf, false)
			if r != nil {
				err = r
			}
			w.WriteHeader(err.HTTPCode)
		}

		buf.WriteTo(w)
		w.Write([]byte("\n\n"))
	}()

	switch method {
	case "GET":
		resource, err = find(path)

	case "POST":
		if len(path) != 1 {
			err = notaryapi.CreateError(notaryapi.ErrorBadMethod, `POST can only be used in the root context: /v1`)
			return
		}

		resource, err = post("/"+strings.Join(path, "/"), form)

	default:
		err = notaryapi.CreateError(notaryapi.ErrorBadMethod, fmt.Sprintf(`The HTTP %s method is not supported`, method))
		return
	}

	if err != nil {
		resource = err
	}

	alt := false
	for _, s := range form["byref"] {
		b, err := strconv.ParseBool(s)
		if err == nil {
			alt = b
			break
		}
	}

	err = notaryapi.Marshal(resource, accept, &buf, alt)
}

var blockPtrType = reflect.TypeOf((*notaryapi.Block)(nil)).Elem()

func post(context string, form url.Values) (interface{}, *notaryapi.Error) {
	newEntry := notaryapi.NewDataEntry([]byte{})
	format, data := form.Get("format"), form.Get("data")

	switch format {
	case "", "json":
		reader := gocoding.ReadString(data)
		err := notaryapi.UnmarshalJSON(reader, newEntry)
		if err != nil {
			return nil, notaryapi.CreateError(notaryapi.ErrorJSONUnmarshal, err.Error())
		}

	case "xml":
		err := xml.Unmarshal([]byte(data), newEntry)
		if err != nil {
			return nil, notaryapi.CreateError(notaryapi.ErrorXMLUnmarshal, err.Error())
		}

	default:
		return nil, notaryapi.CreateError(notaryapi.ErrorUnsupportedUnmarshal, fmt.Sprintf(`The format "%s" is not supported`, format))
	}

	if newEntry == nil {
		return nil, notaryapi.CreateError(notaryapi.ErrorInternal, `Entity to be POSTed is nil`)
	}

	newEntry.StampTime()

	blockMutex.Lock()
	err := blocks[len(blocks)-1].AddEntry(newEntry)
	blockMutex.Unlock()

	if err != nil {
		return nil, notaryapi.CreateError(notaryapi.ErrorInternal, fmt.Sprintf(`Error while adding Entity to Block: %s`, err.Error()))
	}

	return newEntry, nil
}