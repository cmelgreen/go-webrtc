package main

import (
	"html/template"
	"log"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

var tpl *template.Template

func init() {
	tpl = template.Must(template.ParseGlob("../frontend/build/*.html"))
}

func main() {
	router := httprouter.New()

	router.GET("/", index)
	router.HandlerFunc(http.MethodGet, "/ws", webRTCHandle)
	router.ServeFiles("/static/*filepath", http.Dir("../frontend/build/static"))

	log.Fatal(http.ListenAndServe(":8080", router))
}

func index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	tpl.ExecuteTemplate(w, "index.html", nil)
}