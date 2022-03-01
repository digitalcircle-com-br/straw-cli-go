package strawcligo

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type ConnMsg struct {
	Id string `json:"id"`
}

type ReqMsg struct {
	Id   string `json:"id"`
	Data []byte `json:"data"`
	Err  string `json:"err"`
}

type MemoryResponseWriter struct {
	buf     *bytes.Buffer `json:"-"`
	Data    []byte        `json:"data"`
	Status  int           `json:"status"`
	Headers http.Header   `json:"header"`
}

func (m *MemoryResponseWriter) Write(b []byte) (int, error) {
	return m.buf.Write(b)
}
func (m *MemoryResponseWriter) WriteHeader(statusCode int) {
	m.Status = statusCode
}

func (m *MemoryResponseWriter) Header() http.Header {
	return m.Headers
}

func (m *MemoryResponseWriter) Fwd(w http.ResponseWriter) {
	for k, v := range m.Headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(m.Status)
	w.Write(m.Data)
}
func (m *MemoryResponseWriter) Bytes() []byte {
	return m.buf.Bytes()
}

func (m *MemoryResponseWriter) JsonBytes() ([]byte, error) {
	return json.Marshal(m)
}

func (m *MemoryResponseWriter) FromJsonBytes(bs []byte) error {
	return json.Unmarshal(bs, m)
}
func (m *MemoryResponseWriter) Close() {
	if m.Status == 0 {
		m.Status = http.StatusOK
	}
	m.Data = m.buf.Bytes()
}

func NewMemoryResponseWriter() *MemoryResponseWriter {
	ret := &MemoryResponseWriter{buf: &bytes.Buffer{}}
	ret.Headers = http.Header{}
	return ret
}

var router = mux.NewRouter()
var lid string

func handle(c *websocket.Conn) error {
	ci := &ReqMsg{}
	err := c.ReadJSON(ci)
	if err != nil {
		return err
	}
	bi := bufio.NewReader(bytes.NewReader(ci.Data))
	req, err := http.ReadRequest(bi)
	if err != nil {
		return err
	}
	res := NewMemoryResponseWriter()

	res.Header().Add("X-STRAW-RESID", ci.Id)
	res.Header().Add("X-STRAW-EPID", lid)

	router.ServeHTTP(res, req)
	res.Close()
	out := &ReqMsg{}
	out.Id = ci.Id

	if err != nil {
		out.Err = err.Error()

	} else {
		out.Data, err = res.JsonBytes()
		if err != nil {
			log.Printf("Error marshalling response: %s", err.Error())
		}
	}

	err = c.WriteJSON(out)
	if err != nil {
		return err
	}

	return nil

}

func serveOnce(o *Opts) {
	url := o.Url + fmt.Sprintf("?id=%s&to=%v", o.Ep, o.To)
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Printf("Error connecting: %s", err.Error())
		return
	}
	c.SetCloseHandler(func(code int, text string) error {
		log.Printf("Websocket closed: %v - %s", code, text)
		c = nil
		return nil
	})
	defer func() {
		r := recover()
		if r != nil {
			log.Printf("Recovering: %v", r)
		}
		err := c.Close()
		if err != nil {
			log.Printf("Error closing at defer: %s", err.Error())
		}
	}()

	rmsg := &ConnMsg{}
	err = c.ReadJSON(rmsg)
	if err != nil {
		log.Printf("Error reading handshake: %s", err.Error())
		time.Sleep(time.Millisecond * 100)
		return
	}
	lid = rmsg.Id
	log.Printf("Connected - listening at: %s", rmsg.Id)

	errCount := 0
	go func() {
		for errCount != -1 {
			time.Sleep(time.Second)
			if errCount > 10 {
				c.Close()
				c = nil
				return
			}
		}
	}()

	for c != nil {
		err = handle(c)
		if err != nil {
			errCount++
			log.Printf("Error handling conn: %s", err.Error())
		}

	}
}

type Opts struct {
	Url string
	Ep  string
	To  string
}

func Serve(o *Opts) error {
	if o.Url == "" {
		o.Url = "wss://straw.digitalcircle.com.br/v2"
	}
	for {
		serveOnce(o)
		time.Sleep(time.Second)
	}

}

func Router() *mux.Router {
	return router
}
