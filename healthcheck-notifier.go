package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/robfig/cron"
)

const templateLiveReloading = true

type healthcheckNotifier struct {
	HipchatProxy string `json:"hipchat-proxy"`
	SMTPServer   string `json:"smtp-server"`
	Apps         []app  `json:"apps"`
	htmlTemplate *template.Template
}

type app struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	Proxy         string `json:"proxy"`
	HipchatRoom   string `json:"hipchat-room"`
	HipchatToken  string `json:"hipchat-token"`
	ToMailAddress string `json:"to-mail-address"`
	StatusCode    int
	Raw           string
	HTTPClient    *http.Client
	Time          string
}

func (hn *healthcheckNotifier) setupCron() {
	for i := 0; i < len(hn.Apps); i++ {
		p := hn.Apps[i].Proxy
		if p != "" {
			proxyURL, err := url.Parse(p)
			if err != nil {
				panic(err)
			}
			hn.Apps[i].HTTPClient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
		} else {
			hn.Apps[i].HTTPClient = http.DefaultClient
		}
	}

	c := cron.New()
	c.AddFunc("*/30 * * * * *", hn.cmd)
	c.Start()
}

func (hn *healthcheckNotifier) startServer() {
	hn.readTemplate()
	var httpServer http.Server
	http.HandleFunc("/", hn.handler)
	port := os.Getenv("PORT") // for cloud foundry
	if port == "" {
		port = "18888"
	}
	log.Println("start http listening : ", port)
	httpServer.Addr = ":" + port
	log.Println(httpServer.ListenAndServe())
}

func (hn *healthcheckNotifier) readTemplate() {

	funcMap := template.FuncMap{"statusColor": func(s int) string {
		if s == 200 {
			return "green"
		} else if s == 0 {
			return "white"
		}
		return "red"
	}}
	if funcMap != nil {
	}

	t, err := template.New("index.html").Funcs(funcMap).ParseFiles("index.html")
	if err != nil {
		panic(err)
	}
	hn.htmlTemplate = t
}

func (hn *healthcheckNotifier) readConfig() {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}
	json.Unmarshal(file, hn)
	fmt.Println(hn)
}

func (hn *healthcheckNotifier) cmd() {
	fmt.Println("I am runnning task.", time.Now())

	var wg sync.WaitGroup

	for i := 0; i < len(hn.Apps); i++ {
		go func(i int) {
			wg.Add(1)
			hn.Apps[i].healthcheck()
			wg.Done()
		}(i)
	}

	wg.Wait()
}

func (hn *healthcheckNotifier) handler(w http.ResponseWriter, r *http.Request) {
	dump, err := httputil.DumpRequest(r, true)
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	fmt.Println(string(dump))

	if templateLiveReloading {
		hn.readTemplate()
	}
	hn.htmlTemplate.Execute(w, hn.Apps)
}

func (app *app) healthcheck() {
	resp, err := app.HTTPClient.Get(app.URL)
	if err != nil || resp.StatusCode != 200 {
	}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	app.Raw = string(bodyBytes)

	if app.StatusCode != 0 {
		if app.StatusCode != 200 && resp.StatusCode == 200 {
			// up
			hipchat(app)
			mail(app)
		} else if app.StatusCode == 200 && resp.StatusCode != 200 {
			// down
			hipchat(app)
			mail(app)
		}
	}

	app.StatusCode = resp.StatusCode
	app.Time = time.Now().String()
	log.Println(resp.StatusCode)
}

func hipchat(a *app) {
}

func mail(a *app) {
}

func main() {
	var hn healthcheckNotifier
	hn.readConfig()
	hn.setupCron()
	hn.startServer()
}
