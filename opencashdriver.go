package main

import (
	"time"

	"net/http"

	"github.com/gin-gonic/gin"
)

//OpenCashDrawer открыть денежный ящик номер ящика 0,1
func OpenCashDrawer(c *gin.Context) {
	deviceID := c.Param("DeviceID")

	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}

	if kkm.ChkBusy(0) {
		c.XML(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		return
	}
	//займем ккм
	procid := int(time.Now().Unix())
	kkm.SetBusy(procid)
	//освободим по завершению
	defer kkm.SetBusy(0)
	admpass := kkm.GetAdminPass()
	param := make([]byte, 5)
	copy(param, admpass[:4])
	param[5] = byte(0)
	errcode, _, _ := kkm.SendCommand(0x28, param)
	if errcode > 0 {
		c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
	} else {
		c.XML(http.StatusOK, gin.H{"error": false, "message": "ok"})
	}
}
