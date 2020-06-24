// +build riotweb

package embed

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/tidwall/sjson"
)

// From within the Riot Web directory:
// go run github.com/mjibson/esc -o /path/to/dendrite/internal/embed/fs_riotweb.go -private -pkg embed .

func Embed(rootMux *mux.Router, listenPort int, serverName string) {
	url := fmt.Sprintf("http://localhost:%d", listenPort)
	embeddedFS := _escFS(false)
	embeddedServ := http.FileServer(embeddedFS)

	rootMux.Handle("/", embeddedServ)
	rootMux.HandleFunc("/config.json", func(w http.ResponseWriter, _ *http.Request) {
		configFile, err := embeddedFS.Open("/config.sample.json")
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, "Couldn't open the file: "+err.Error())
			return
		}
		configFileInfo, err := configFile.Stat()
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, "Couldn't stat the file: "+err.Error())
			return
		}
		buf := make([]byte, configFileInfo.Size())
		n, err := configFile.Read(buf)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, "Couldn't read the file: "+err.Error())
			return
		}
		if int64(n) != configFileInfo.Size() {
			w.WriteHeader(500)
			io.WriteString(w, "The returned file size didn't match what we expected")
			return
		}
		js, _ := sjson.SetBytes(buf, "default_server_config.m\\.homeserver.base_url", url)
		js, _ = sjson.SetBytes(js, "default_server_config.m\\.homeserver.server_name", serverName)
		js, _ = sjson.SetBytes(js, "brand", fmt.Sprintf("Riot %s", serverName))
		js, _ = sjson.SetBytes(js, "disable_guests", true)
		js, _ = sjson.SetBytes(js, "disable_3pid_login", true)
		js, _ = sjson.DeleteBytes(js, "welcomeUserId")
		_, _ = w.Write(js)
	})

	fmt.Println("*-------------------------------*")
	fmt.Println("| This build includes Riot Web! |")
	fmt.Println("*-------------------------------*")
	fmt.Println("Point your browser to:", url)
	fmt.Println()
}
