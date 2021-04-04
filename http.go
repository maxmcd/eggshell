package main

import (
	"context"
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

//go:embed index.html
var indexFileContents []byte

type Change struct {
	Row      int
	Column   int
	OldValue string
	NewValue string
}

func R(fn func(*gin.Context) error) func(*gin.Context) {
	return func(c *gin.Context) {
		if err := fn(c); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"msg": err.Error()})
		}
	}
}

func (s *Sheet) RunServer(ctx context.Context, onChange chan struct{}, addr string) *http.Server {
	router := gin.Default()
	router.GET("/", R(func(c *gin.Context) error {
		c.Writer.Header().Add("Content-Type", "text/html")
		_, _ = c.Writer.Write(indexFileContents)
		return nil
	}))

	router.GET("/data.json", R(func(c *gin.Context) error {
		c.JSON(http.StatusOK, s.grid)
		return nil
	}))

	router.POST("/update", R(func(c *gin.Context) error {
		var grid Grid
		if err := c.BindJSON(&grid); err != nil {
			return err
		}
		s.grid = grid
		_ = s.WriteConfig("./eggshell.csv")
		onChange <- struct{}{}
		return nil
	}))

	router.POST("/change", R(func(c *gin.Context) error {
		var changes []Change
		if err := c.BindJSON(&changes); err != nil {
			return err
		}
		for _, change := range changes {
			s.grid[change.Row][change.Column] = change.NewValue
		}
		_ = s.WriteConfig("./eggshell.csv")
		onChange <- struct{}{}
		return nil
	}))
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}
	var done bool
	go func() {
		if err := server.ListenAndServe(); err != nil && !done {
			panic(errors.Wrap(err, "server closed unexpectedly"))
		}
	}()
	go func() {
		<-ctx.Done()
		done = true
		// use cancelled context so we'll shut down immediately
		_ = server.Shutdown(ctx)
	}()
	return server
}
