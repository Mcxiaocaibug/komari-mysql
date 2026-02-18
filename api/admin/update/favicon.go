package update

import (
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/api"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
)

func UploadFavicon(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5<<20) // 5MB
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			api.RespondError(c, http.StatusRequestEntityTooLarge, "File too large. Maximum size is 5MB")
		} else {
			api.RespondError(c, http.StatusBadRequest, err.Error())
		}
		return
	}
	if err := os.WriteFile("./data/favicon.ico", data, 0644); err != nil {
		api.RespondError(c, http.StatusInternalServerError, "Failed to save favicon: "+err.Error())
		return
	}

	// 数据库持久化
	db := dbcore.GetDBInstance()
	staticFile := models.StaticFile{
		Name: "favicon.ico",
		Data: data,
	}
	if err := db.Save(&staticFile).Error; err != nil {
		api.RespondError(c, http.StatusInternalServerError, "Failed to save favicon to DB: "+err.Error())
		return
	}

	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "Favicon uploaded", "info")
	api.RespondSuccess(c, nil)
}

func DeleteFavicon(c *gin.Context) {
	if err := os.Remove("./data/favicon.ico"); err != nil {
		if os.IsNotExist(err) {
			api.RespondError(c, http.StatusNotFound, "Favicon not found")
		} else {
			api.RespondError(c, http.StatusInternalServerError, "Failed to delete favicon: "+err.Error())
		}
		return
	}

	// 数据库删除
	db := dbcore.GetDBInstance()
	db.Delete(&models.StaticFile{}, "name = ?", "favicon.ico")

	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "Favicon deleted", "info")
	api.RespondSuccess(c, nil)
}
