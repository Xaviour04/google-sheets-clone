package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	_ "github.com/mattn/go-sqlite3"
)

func ExitOn(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

const (
	host string = "localhost"
	port int    = 9876
)

var (
	websocketServer *WebsocketServer
	HTTPserver      *http.Server
	db              *sql.DB
)

type tableConfig struct {
	ID            string
	Title         string
	Cols          int
	Rows          int
	DefaultWidth  int
	WidthChanges  map[int]int
	DefaultHeight int
	HeightChanges map[int]int
}

func HandleHomePage(w http.ResponseWriter, r *http.Request) {
	fileID := uuid.NewString()
	http.Redirect(w, r, "/open/"+fileID, http.StatusTemporaryRedirect)

}

func HandleConnectAPI(w http.ResponseWriter, r *http.Request) {
	ID, ok := mux.Vars(r)["id"]
	if !ok {
		http.NotFound(w, r)
		return
	}

	result := db.QueryRow(`SELECT name FROM sqlite_master WHERE type="table" AND name=?;`, ID)

	switch result.Err() {
	case nil:
		serveWs(websocketServer, w, r, ID)
	case sql.ErrNoRows:
		http.Redirect(w, r, "/", http.StatusNotFound)
	default:
		http.Redirect(w, r, "/", http.StatusInternalServerError)
	}

}

func marshalAndRenderOpen(config tableConfig, w http.ResponseWriter, r *http.Request) {
	marshalled, err := json.Marshal(config)
	if err != nil {
		log.Printf("[ERROR] %q occured while marshalling newly created config data config", err.Error())
		log.Printf("[ ... ] URL = %q", r.URL.Path)
		http.Redirect(w, r, "/", http.StatusInternalServerError)
		return
	}

	html, err := template.ParseFiles("frontend/build/index.html")
	if err != nil {
		log.Printf("[ERROR] %q occured while parsing html file", err.Error())
		log.Printf("[ ... ] URL = %q", r.URL.Path)
		log.Printf("[ ... ] dir = %q", "frontend/build/index.html")
		http.Redirect(w, r, "/", http.StatusInternalServerError)
		return
	}

	err = html.Execute(w, string(marshalled))
	if err != nil {
		log.Printf("[ERROR] %q occured while executing templating", err.Error())
		log.Printf("[ ... ] URL = %q", r.URL.Path)
		log.Printf("[ ... ] marshalled data = %q", marshalled)
		http.Redirect(w, r, "/", http.StatusInternalServerError)
		return
	}
}

func HandleOpenFile(w http.ResponseWriter, r *http.Request) {
	ID, ok := mux.Vars(r)["id"]
	if !ok {
		http.NotFound(w, r)
		return
	}

	result := db.QueryRow(`SELECT * FROM config WHERE id=?`, ID)
	config := tableConfig{}
	var widthChanges string
	var heightChanges string
	err := result.Scan(&config.ID, &config.Title, &config.Cols, &config.Rows, &config.DefaultWidth, &widthChanges, &config.DefaultHeight, &heightChanges)

	if err == nil {
		json.Unmarshal([]byte(widthChanges), &(config.WidthChanges))
		json.Unmarshal([]byte(heightChanges), &(config.HeightChanges))

		marshalAndRenderOpen(config, w, r)
		return
	}

	if err != sql.ErrNoRows {
		log.Printf("[ERROR] %q occured while executing query to find if requested table adready exists", err.Error())
		log.Panicf("[ ... ] FileID = %q", ID)

		http.Redirect(w, r, "/", http.StatusInternalServerError)
		return
	}

	config = tableConfig{ID, "Untitled", 26, 100, 96, map[int]int{}, 32, map[int]int{}}

	// id title rows cols default_width widthChanges default_height height_changes
	widthChanges_bytes, _ := json.Marshal(config.WidthChanges)   //TODO: do something about the error
	heightChanges_bytes, _ := json.Marshal(config.HeightChanges) //TODO: do something about the error

	_, err = db.Exec(
		`INSERT INTO config VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		config.ID,
		config.Title,
		config.Rows,
		config.Cols,
		config.DefaultWidth,
		string(widthChanges_bytes),
		config.DefaultHeight,
		string(heightChanges_bytes),
	)

	if err != nil {
		log.Printf("[ERROR] %q occured while executing query to create a new config record", err.Error())
		log.Panicf("[ ... ] FileID = %q", ID)

		http.Redirect(w, r, "/", http.StatusInternalServerError)
		return
	}

	cols_str := ""
	for i := 0; i < config.Cols; i++ {
		cols_str += fmt.Sprintf("\"%d\" TEXT NOT NULL, ", i)
	}
	cols_str = strings.TrimRight(cols_str, ", ")
	query := fmt.Sprintf("CREATE TABLE %q (row INTEGER, %s)", ID, cols_str)
	_, err = db.Exec(query)

	if err != nil {
		log.Printf("[ERROR] %q occured while executing query to create a new table", err.Error())
		log.Panicf("[ ... ] query = %q", query)

		http.Redirect(w, r, "/", http.StatusInternalServerError)
		return
	}

	for row := 0; row < config.Rows; row++ {
		cols_str := strings.Repeat("\"\", ", config.Cols)
		cols_str = strings.TrimSuffix(cols_str, ", ")
		query := fmt.Sprintf("INSERT INTO %q VALUES(%d, %s)", ID, row, cols_str)
		_, err = db.Exec(query)

		if err != nil {
			log.Printf("[ERROR] %q occured while executing query to create a new table", err.Error())
			log.Panicf("[ ... ] query = %q", query)

			http.Redirect(w, r, "/", http.StatusInternalServerError)
			return
		}
	}

	marshalAndRenderOpen(config, w, r)
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/", HandleHomePage)
	router.HandleFunc("/open/{id}", HandleOpenFile)
	router.HandleFunc("/connect-ws/{id}", HandleConnectAPI)
	router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "frontend/build/"+r.URL.Path)
	})

	log.Printf("http://%s:%d", host, port)

	var err error
	db, err = sql.Open("sqlite3", "google-sheets.db")
	ExitOn(err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS config (
		id TEXT NOT NULL PRIMARY KEY,
		title TEXT NOT NULL,
		rows INTEGER NOT NULL,
		cols INTEGER NOT NULL,
		default_width INTEGER NOT NULL,
		width_changes BLOB NOT NULL,
		default_height INTEGER NOT NULL,
		height_changes BLOB NOT NULL
	);`)
	ExitOn(err)

	websocketServer = createWebsocketServer()
	HTTPserver = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: router}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go websocketServer.run()
	go HTTPserver.ListenAndServe()

	log.Println("Main thread is waiting for exit signal")
	sig := <-sigs
	log.Printf("Shutting server down because of %q", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ExitOn(HTTPserver.Shutdown(ctx))
}
