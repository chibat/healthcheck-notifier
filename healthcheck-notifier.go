package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/robfig/cron"
)

const templateLiveReloading = true

// HealthcheckNotifier HealthcheckNotifier
type HealthcheckNotifier struct {
	Cron             string `json:"cron"`
	HipchatProxy     string `json:"hipchat-proxy"`
	HipchatSubdomain string `json:"hipchat-subdomain"`
	SMTPServer       string `json:"smtp-server"`
	MailAddressFrom  string `json:"mail-address-from"`
	Apps             []App  `json:"apps"`
	htmlTemplate     *template.Template
	HipChatClient    *http.Client
}

// App target app
type App struct {
	Name                string   `json:"name"`
	URL                 string   `json:"url"`
	Proxy               string   `json:"proxy"`
	HipchatRoom         string   `json:"hipchat-room"`
	HipchatToken        string   `json:"hipchat-token"`
	MailAddressToDown   []string `json:"mail-address-to-down"`
	MailAddressToUp     []string `json:"mail-address-to-up"`
	StatusCode          int
	Raw                 string
	HTTPClient          *http.Client
	Time                string
	HealthcheckNotifier *HealthcheckNotifier
}

type hipchatRequest struct {
	Notify        bool   `json:"notify"`
	MessageFormat string `json:"message_format"`
	Color         string `json:"color"`
	Message       string `json:"message"`
}

// Init init
func (hn *HealthcheckNotifier) Init() {
	for i := 0; i < len(hn.Apps); i++ {
		hn.Apps[i].HealthcheckNotifier = hn
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

	if hn.HipchatProxy != "" {
		proxyURL, err := url.Parse(hn.HipchatProxy)
		if err != nil {
			panic(err)
		}
		hn.HipChatClient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	} else {
		hn.HipChatClient = http.DefaultClient
	}

	c := cron.New()
	c.AddFunc(hn.Cron, hn.cmd)
	c.Start()
}

// StartServer start server
func (hn *HealthcheckNotifier) StartServer() {
	hn.readTemplate()
	var httpServer http.Server
	http.HandleFunc("/", hn.handler)
	http.HandleFunc("/test-internal-server-error", testHandlerInternalServerError) // for test
	port := os.Getenv("PORT")                                                      // for cloud foundry
	if port == "" {
		port = "18888"
	}
	log.Println("start http listening : ", port)
	httpServer.Addr = ":" + port
	httpServer.ListenAndServe()
}

func (hn *HealthcheckNotifier) readTemplate() {

	funcMap := template.FuncMap{"statusColor": func(s int) string {
		if s == 200 {
			return "green"
		} else if s == 0 {
			return "white"
		}
		return "red"
	}}

	// t, err := template.New("template.html").Funcs(funcMap).ParseFiles("template-debug.html") // for html debug
	t, err := template.New("template").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		panic(err)
	}
	hn.htmlTemplate = t
}

// ReadConfig read config
func (hn *HealthcheckNotifier) ReadConfig() {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}
	json.Unmarshal(file, hn)
}

func (hn *HealthcheckNotifier) cmd() {

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

func (hn *HealthcheckNotifier) handler(w http.ResponseWriter, r *http.Request) {
	if templateLiveReloading {
		hn.readTemplate()
	}
	hn.htmlTemplate.Execute(w, hn.Apps)
}

var testHandlerInternalServerErrorFlag = true

func testHandlerInternalServerError(w http.ResponseWriter, r *http.Request) {
	if testHandlerInternalServerErrorFlag {
		http.Error(w, "test error", http.StatusInternalServerError)
	}
	testHandlerInternalServerErrorFlag = !testHandlerInternalServerErrorFlag
}

func (a *App) healthcheck() {
	resp, err := a.HTTPClient.Get(a.URL)
	var statusCode int
	if err != nil {
		statusCode = -1
	} else {
		statusCode = resp.StatusCode
	}
	// bodyBytes, err := ioutil.ReadAll(resp.Body)
	// a.Raw = string(bodyBytes)

	message := "healthcheck error"
	if statusCode == 200 {
		message = "healthcheck recovered"
	}

	body := fmt.Sprintf(`
Message: %s
App: %s
Status Code: %d
URL: %s`, message, a.Name, statusCode, a.URL)

	if (a.StatusCode != 200 && a.StatusCode != 0 && statusCode == 200) || ((a.StatusCode == 200 || a.StatusCode == 0) && statusCode != 200) {
		a.NotifyWithHipchat(body, statusCode)
		a.NotifyWithMail(body, statusCode)
	}

	a.StatusCode = statusCode
	a.Time = time.Now().Format(time.RFC3339)
	log.Printf("%s: %d", a.Name, statusCode)
}

// NotifyWithHipchat notify with hipchat
func (a *App) NotifyWithHipchat(body string, statusCode int) {
	if a.HipchatRoom == "" || a.HipchatToken == "" {
		return
	}

	url := "https://api.hipchat.com/v2/room/" + a.HipchatRoom + "/notification?auth_token=" + a.HipchatToken

	color := "red"
	if statusCode == 200 {
		color = "green"
	}
	input, err := json.Marshal(&hipchatRequest{Notify: true, MessageFormat: "text", Color: color, Message: "@all\n" + body})
	if err != nil {
		log.Print(err)
		return
	}
	resp, err := a.HealthcheckNotifier.HipChatClient.Post(url, "application/json", bytes.NewBuffer(input))
	if err != nil {
		log.Print(err)
		return
	}
	log.Printf("%s HipChat: %d", a.Name, resp.StatusCode)
}

// NotifyWithMail notify with mail
func (a *App) NotifyWithMail(body string, statusCode int) {
	from := a.HealthcheckNotifier.MailAddressFrom
	server := a.HealthcheckNotifier.SMTPServer
	to := a.MailAddressToDown
	subject := "[DOWN] " + a.Name
	if statusCode == 200 {
		to = a.MailAddressToUp
		subject = "[UP] " + a.Name
	}
	if server == "" || from == "" || len(to) == 0 {
		return
	}

	msg := "From: " + from + "\r\n" +
		"To: " + toLine(to) + "\r\n" +
		"Subject: " + subject + "\r\n\r\n" +
		body + "\r\n"

	err := smtp.SendMail(server, nil, from, to, []byte(msg))
	if err != nil {
		log.Print(err)
		return
	}
}

func toLine(array []string) string {
	var ret string
	for i, s := range array {
		if i == 0 {
			ret = s
		} else {
			ret = ret + ", " + s
		}
	}
	return ret
}

func main() {
	var notifier HealthcheckNotifier
	notifier.ReadConfig()
	notifier.Init()
	notifier.StartServer()
}

const htmlTemplate = `
<html>
<head>
<title>Healthcheck Notifier</title>
<meta http-equiv="refresh" content="60">
<style type="text/css">
th {padding: 10px}
td {padding: 10px}
</style>
</head>
<body>
<h1>Healthcheck Notifier</h1>
<table style="border-collapse: collapse;" border="1">
<tr style="background-color: darkgray">
  <th>app name</th>
  <th>status</th>
  <th>hipchat</th>
  <th>mail(down)</th>
  <th>mail(up)</th>
  <th>time</th>
</tr>
{{range .}}
<tr>
  <td><a style="text-decoration: none;" href="{{.URL}}" target="_blank">{{.Name}}</a></td>
  <td style="color: white; background-color: {{.StatusCode | statusColor}}">{{.StatusCode}}</td>
  <td><a href="https://{{.HealthcheckNotifier.HipchatSubdomain}}.hipchat.com/chat/room/{{.HipchatRoom}}" target="_blank">{{.HipchatRoom}}</a></td>
  <td>{{.MailAddressToDown}}</td>
  <td>{{.MailAddressToUp}}</td>
  <td>{{.Time}}</td>
</tr>
{{end}}
</table>
</body>
</html>
`
