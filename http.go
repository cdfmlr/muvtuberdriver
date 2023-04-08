package main

import (
	"bytes"
	"encoding/json"
	"muvtuberdriver/model"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/exp/slog"
)

// TextInFromHTTP listen addr, wait TextIn from requests and send them to textInChan:
//
//	POST routePath
//	Content-Type: application/json
//	{ "author": "author", "content": "content" }
//
// routePath is the path of the route, default is "/".
func TextInFromHTTP(addr string, routePath string, textInChan chan<- *model.TextIn) {
	if strings.TrimSpace(routePath) == "" {
		routePath = "/"
	}

	// no logger
	r := gin.New()
	r.Use(gin.Recovery())

	r.POST(routePath, func(c *gin.Context) {
		var textIn model.TextIn
		if err := c.BindJSON(&textIn); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		slog.Info("[TextInFromHTTP] recv TextIn from HTTP.", "author", textIn.Author, "priority", textIn.Priority, "content", textIn.Content)
		textInChan <- &textIn
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.Run(addr)
}

func TextOutToHttp(addr string, textOut *model.TextOut) {
	if addr == "" {
		return
	}
	j, err := json.Marshal(textOut)
	if err != nil {
		slog.Error("[TextOutToHttp] marshal json error", "err", err)
		return
	}
	http.Post(addr, "application/json", bytes.NewReader(j))
}

func init() {
	gin.SetMode(gin.ReleaseMode)
}
