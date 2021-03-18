package main

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
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

func (s *Sheet) RunServer(addr string) (err error) {
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
		return nil
	}))

	router.POST("/change", R(func(c *gin.Context) error {
		var changes []Change
		if err := c.BindJSON(&changes); err != nil {
			return err
		}
		for _, change := range changes {
			s.UpdateCell(change.Row, change.Column, change.NewValue)
			s.grid[change.Row][change.Column] = change.NewValue
		}
		_ = s.WriteConfig("./eggshell.csv")
		return nil
	}))
	return router.Run(addr)
}
