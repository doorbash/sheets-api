package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/mux"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/sheets/v4"
)

const (
	SPREADSHEET      = "PUT-YOUR-SPREADSHEET-ID-HERE"
	CREDENTIALS_FILE = "credentials.json"
	tokFile          = "token.json"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", homeHandler)
	r.HandleFunc("/login", loginHandler)
	r.HandleFunc("/callback", callbackHandler)
	r.HandleFunc("/{sheet}/get", sheetHandler)
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
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
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
		saveToken(tokFile, tok)
		rw.WriteHeader(200)
		rw.Write([]byte("You are logged in!"))
	} else {
		rw.WriteHeader(400)
		rw.Write([]byte("Unable to read authorization code"))
	}
}

func sheetHandler(rw http.ResponseWriter, r *http.Request) {
	var sheet = mux.Vars(r)["sheet"]

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

	tok, err := tokenFromFile(tokFile)
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

	var s map[string]interface{} = make(map[string]interface{})

	for _, row := range resp.Values {
		// fmt.Printf("%s --> ", row[0])

		var key string = row[0].(string)
		var value string = ""

		if len(row) > 1 {
			value = row[1].(string)
		}

		if value == "true" || value == "TRUE" {
			s[key] = true
		} else if value == "false" || value == "FALSE" {
			s[key] = false
		} else if value == "null" {
			s[key] = nil
		} else if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			s[key] = i
		} else if f, err := strconv.ParseFloat(value, 64); err == nil {
			s[key] = f
		} else {
			s[key] = value
		}

		// if len(row) > 1 {
		// 	fmt.Printf("type of %v is %T\n\n", row[1].(string), s[key])
		// } else {
		// 	fmt.Printf("type of %v is %T\n\n", s[key], s[key])
		// }

	}

	query := r.URL.Query()
	if len(query) == 0 {
		data, err := json.Marshal(s)
		if err != nil {
			rw.WriteHeader(400)
			rw.Write([]byte(fmt.Sprintf("Error: %v", err)))
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(200)
		rw.Write(data)
	} else {
		if _, ok := query["key"]; ok {
			if _, ok := s[query["key"][0]]; ok {
				rw.WriteHeader(200)
				rw.Write([]byte(fmt.Sprintf("%v", s[query["key"][0]])))
			} else {
				rw.WriteHeader(400)
				rw.Write([]byte(fmt.Sprintf("Error: %s is not specified in sheet.", query["key"][0])))
			}
		} else {
			rw.WriteHeader(400)
			rw.Write([]byte("Error: key= is not in url."))
		}
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
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}
