package main

import (
	_ "embed"
	"html/template"
	"log"
	"net/http"
	"time"
)

//go:embed home.md
var homeMarkdown string

const homeAnnotationsJSON = `[
  {"id":1,"author":"Sam","comment":"This is the whole pitch. Love it.","quote":"One API call. One short URL.","text_start":243,"text_end":271,"created":1747267200},
  {"id":2,"author":"Priya","comment":"Zero friction for the viewer is the key unlock here","quote":"No account, no login, no viewer setup.","text_start":272,"text_end":310,"created":1747353600},
  {"id":3,"author":"Marcus","comment":"Night and day difference honestly","quote":"Clean rendered page","text_start":696,"text_end":715,"created":1747440000},
  {"id":4,"author":"Sam","comment":"Smart. No API keys to leak, no auth service to run.","quote":"proof-of-work","text_start":645,"text_end":658,"created":1747526400},
  {"id":5,"author":"Jules","comment":"Under a second is fine. The agent is already spending 30s thinking anyway","quote":"your agent computes a SHA-256 partial collision locally in under a second","text_start":888,"text_end":961,"created":1747612800},
  {"id":6,"author":"Priya","comment":"This is what makes it sticky. People come back to comment.","quote":"Highlight + comment","text_start":737,"text_end":756,"created":1747699200},
  {"id":7,"author":"Marcus","comment":"Exactly the right comparison. Except you don't need a Google account.","quote":"like Google Docs comments but for a page that took one second to create","text_start":1486,"text_end":1557,"created":1747785600},
  {"id":8,"author":"Jules","comment":"This is the self-host story I want. One binary, done.","quote":"Single Go binary, SQLite storage, zero external dependencies","text_start":1624,"text_end":1684,"created":1747872000},
  {"id":9,"author":"Sam","comment":"Keep it this way. Simplicity is the moat.","quote":"under 2000 lines of Go","text_start":1799,"text_end":1821,"created":1747958400}
]`

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	html, err := s.renderer.Render([]byte(homeMarkdown))
	if err != nil {
		log.Printf("home render error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	page := pitchPage{
		Title:           "pitchbin",
		Author:          "sinitax",
		HTML:            template.HTML(html),
		Created:         time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
		Views:           0,
		ID:              "",
		BaseURL:         s.baseURL,
		Readonly:        true,
		AnnotationsJSON: template.JS(homeAnnotationsJSON),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.Execute(w, page); err != nil {
		log.Printf("template error: %v", err)
	}
}
