package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/sheets/v4"
)

const (
	SPREADSHEET            = "PUT-YOUR-SPREADSHEET-ID-HERE"
	CREDENTIALS_FILE       = "credentials.json"
	TOKEN_FILE             = "token.json"
	TOKEN_REFRESH_INTERVAL = 30 * 60 * time.Second
	CACHE_DATA             = false
	CACHE_INTERVAL         = 5 * 60 * time.Second
)

var configData map[string]map[string]interface{} = make(map[string]map[string]interface{})
var lastConfigGetTime map[string]time.Time = make(map[string]time.Time)

func main() {
	go func() {
		time.Sleep(60 * time.Second)
		for {
			refreshToken()
			time.Sleep(TOKEN_REFRESH_INTERVAL)
		}
	}()
	r := mux.NewRouter()
	r.HandleFunc("/", homeHandler)
	r.HandleFunc("/login", loginHandler)
	r.HandleFunc("/callback", callbackHandler)
	r.HandleFunc("/{sheet}", sheetHandler)
	http.Handle("/", r)
	err := http.ListenAndServe(":4040", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func homeHandler(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(200)
	rw.Write([]byte("It's working!"))
}

func loginHandler(rw http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadFile(CREDENTIALS_FILE)
	if err != nil {
		rw.WriteHeader(400)
		rw.Write([]byte(fmt.Sprintf("Unable to read client secret file: %v", err)))
		return
	}
	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets.readonly")
	if err != nil {
		rw.WriteHeader(400)
		rw.Write([]byte(fmt.Sprintf("Unable to parse client secret file to config: %v", err)))
		return
	}
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("approval_prompt", "force"))
	//fmt.Printf("auth url is %s\n", authURL)
	http.Redirect(rw, r, authURL, 301)
}

func callbackHandler(rw http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if _, ok := query["code"]; ok {
		authCode := query["code"][0]

		// fmt.Printf("auth code is %s\n", authCode)

		b, err := ioutil.ReadFile(CREDENTIALS_FILE)
		if err != nil {
			rw.WriteHeader(400)
			rw.Write([]byte(fmt.Sprintf("Unable to read client secret file: %v", err)))
			return
		}
		config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets.readonly")
		if err != nil {
			rw.WriteHeader(400)
			rw.Write([]byte(fmt.Sprintf("Unable to parse client secret file to config: %v", err)))
			return
		}
		tok, err := config.Exchange(context.TODO(), authCode)
		if err != nil {
			rw.WriteHeader(400)
			rw.Write([]byte(fmt.Sprintf("Unable to retrieve token from web: %v", err)))
			return
		}
		//fmt.Printf("token is %v\n", tok)
		saveToken(TOKEN_FILE, tok)
		rw.WriteHeader(200)
		rw.Write([]byte("You are logged in!"))
	} else {
		rw.WriteHeader(400)
		rw.Write([]byte("Unable to read authorization code"))
	}
}

func sheetHandler(rw http.ResponseWriter, r *http.Request) {
	var sheet = mux.Vars(r)["sheet"]
	//var isCached bool = true

	if _, ok := lastConfigGetTime[sheet]; !CACHE_DATA || !ok || time.Now().Sub(lastConfigGetTime[sheet]) >= CACHE_INTERVAL {
		b, err := ioutil.ReadFile(CREDENTIALS_FILE)
		if err != nil {
			rw.WriteHeader(400)
			rw.Write([]byte(fmt.Sprintf("Unable to read client secret file: %v", err)))
			return
		}

		config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets.readonly")
		if err != nil {
			rw.WriteHeader(400)
			rw.Write([]byte(fmt.Sprintf("Unable to parse client secret file to config: %v", err)))
			return
		}

		tok, err := tokenFromFile(TOKEN_FILE)
		if err != nil {
			rw.WriteHeader(400)
			rw.Write([]byte(fmt.Sprintf("Error: %v", err)))
			return
		}
		client := config.Client(context.Background(), tok)

		srv, err := sheets.New(client)
		if err != nil {
			rw.WriteHeader(400)
			rw.Write([]byte(fmt.Sprintf("Unable to retrieve Sheets client: %v", err)))
			return
		}

		resp, err := srv.Spreadsheets.Values.Get(SPREADSHEET, sheet+"!A:B").Do()
		if err != nil {
			rw.WriteHeader(400)
			rw.Write([]byte(fmt.Sprintf("Unable to retrieve data from sheet: %v", err)))
			return
		}

		configData[sheet] = make(map[string]interface{})

		for _, row := range resp.Values {
			// fmt.Printf("%s --> ", row[0])

			if len(row) == 0 {
				continue
			}

			var key string = row[0].(string)
			var value string = ""

			if len(row) > 1 {
				value = row[1].(string)
			}

			if value == "true" || value == "TRUE" {
				configData[sheet][key] = true
			} else if value == "false" || value == "FALSE" {
				configData[sheet][key] = false
			} else if value == "null" {
				configData[sheet][key] = nil
			} else if i, err := strconv.ParseInt(value, 10, 64); err == nil {
				configData[sheet][key] = i
			} else if f, err := strconv.ParseFloat(value, 64); err == nil {
				configData[sheet][key] = f
			} else {
				configData[sheet][key] = value
			}
		}
		if CACHE_DATA {
			lastConfigGetTime[sheet] = time.Now()
		}
		//isCached = false
	}

	query := r.URL.Query()
	if len(query) == 0 {
		data, err := json.Marshal(configData[sheet])
		if err != nil {
			rw.WriteHeader(400)
			rw.Write([]byte(fmt.Sprintf("Error: %v", err)))
		} else {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(200)
			rw.Write(data)
		}
	} else {
		if _, ok := query["key"]; ok {
			if _, ok := configData[sheet][query["key"][0]]; ok {
				rw.WriteHeader(200)
				rw.Write([]byte(fmt.Sprintf("%v", configData[sheet][query["key"][0]])))
			} else {
				rw.WriteHeader(404)
				rw.Write([]byte(fmt.Sprintf("Error: key %s is not in sheet.", query["key"][0])))
			}
		} else {
			rw.WriteHeader(400)
			rw.Write([]byte("Error: key param is not in url."))
		}
	}

	if !CACHE_DATA {
		delete(configData, sheet)
	}
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	// fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to save oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func refreshToken() {
	tok, err := tokenFromFile(TOKEN_FILE)
	if err != nil {
		fmt.Printf("Error while renewing token %v\n", err)
		return
	}
	b, err := ioutil.ReadFile(CREDENTIALS_FILE)
	if err != nil {
		fmt.Printf("Error while renewing token %v\n", err)
		return
	}
	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets.readonly")
	if err != nil {
		fmt.Printf("Error while renewing token %v\n", err)
		return
	}

	if tok.Expiry.Sub(time.Now()) < TOKEN_REFRESH_INTERVAL+5*60*time.Second {
		urlValue := url.Values{"client_id": {config.ClientID}, "client_secret": {config.ClientSecret}, "refreshToken": {tok.RefreshToken}, "grant_type": {"refreshToken"}}

		resp, err := http.PostForm("https://www.googleapis.com/oauth2/v3/token", urlValue)
		if err != nil {
			fmt.Printf("Error while renewing token %v\n", err)
			return
		}

		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("Error while renewing token %v\n", err)
			return
		}
		//fmt.Printf("body = %s\n", body)
		var refreshToken map[string]interface{}
		json.Unmarshal([]byte(body), &refreshToken)

		// fmt.Printf("refreshToken = %+v\n", refreshToken)

		then := time.Now()
		then = then.Add(time.Duration(refreshToken["expires_in"].(float64)) * time.Second)

		tok.Expiry = then
		tok.AccessToken = refreshToken["access_token"].(string)
		saveToken(TOKEN_FILE, tok)

		fmt.Printf("Access token refreshed\n")
	} else {
		fmt.Printf("No need to renew access token\n")
		fmt.Printf("Access token expires in %v\n", tok.Expiry.Sub(time.Now()))
		fmt.Printf("Next token refresh is in %v\n", TOKEN_REFRESH_INTERVAL)
	}

}
