package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"github.com/golang/glog"
	"github.com/phuslu/goproxy/httpproxy"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
)

const (
	appspotDomain string = "appspot.com"
	goagentPath   string = "/_gh/"
)

type GAERequestFilter struct {
	AppIDs []string
	Schema string
}

func (g *GAERequestFilter) pickAppID() string {
	return g.AppIDs[0]
}

func (g *GAERequestFilter) encodeRequest(req *http.Request) (*http.Request, error) {
	var b bytes.Buffer
	var err error
	w, err := flate.NewWriter(&b, 9)
	defer w.Close()
	if err != nil {
		return nil, err
	}
	_, err = fmt.Fprintf(w, "%s %s %s\r\n", req.Method, req.URL.String(), req.Proto)
	if err != nil {
		return nil, err
	}
	for key, values := range req.Header {
		for _, value := range values {
			_, err := fmt.Fprintf(w, "%s: %s\r\n", key, value)
			if err != nil {
				return nil, err
			}
		}
	}
	_, err = w.Write([]byte("\r\n"))
	if err != nil {
		return nil, err
	}
	err = w.Flush()
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(&b)
	if err != nil {
		return nil, err
	}
	var b1 bytes.Buffer
	err = binary.Write(&b1, binary.BigEndian, int16(len(data)))
	if err != nil {
		return nil, err
	}
	written := int64(2)
	n1, err := b1.Write(data)
	if err != nil {
		return nil, err
	}
	written += int64(n1)
	n2, err := io.Copy(&b1, req.Body)
	if err != nil {
		return nil, err
	}
	written += n2
	url := fmt.Sprintf("%s://%s.%s%s", g.Schema, g.pickAppID(), appspotDomain, goagentPath)
	req1, err := http.NewRequest("POST", url, &b1)
	if err != nil {
		return nil, err
	}
	req1.Header.Add("Conntent-Length", strconv.FormatInt(written, 10))
	return req1, nil
}

func (g *GAERequestFilter) decodeResponse(res *http.Response) (*http.Response, error) {
	if res.StatusCode >= 300 {
		return res, nil
	}
	var length int16
	err := binary.Read(res.Body, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}
	r := bufio.NewReader(flate.NewReader(&io.LimitedReader{res.Body, int64(length)}))
	res1, err := http.ReadResponse(r, res.Request)
	if err != nil {
		return nil, err
	}
	res1.Body = res.Body
	res1.Request = res.Request
	return res1, nil
}

func (g *GAERequestFilter) HandleRequest(h *httpproxy.Handler, args *http.Header, rw http.ResponseWriter, req *http.Request) (*http.Response, error) {
	gaeReq, err := g.encodeRequest(req)
	if err != nil {
		rw.WriteHeader(502)
		fmt.Fprintf(rw, "Error: %s\n", err)
		return nil, err
	}
	gaeReq.Header = req.Header
	res, err := h.Net.HttpClientDo(gaeReq)
	if err == nil {
		glog.Infof("%s \"GAE %s %s %s\" %d %s", req.RemoteAddr, req.Method, req.URL.String(), req.Proto, res.StatusCode, res.Header.Get("Content-Length"))
	}
	return g.decodeResponse(res)
}

func (g *GAERequestFilter) Filter(req *http.Request) (args *http.Header, err error) {
	return nil, nil
}
