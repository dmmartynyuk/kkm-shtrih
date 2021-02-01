package main

import (
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

//CloseShift закрыть смену
func printTextDocument(c *gin.Context) {
	/*
	   <?xml version="1.0" encoding="UTF-8"?>
	   <Document>
	   	<Positions>
	   		<TextString Text="Участие в дисконтной системе"/>
	   		<TextString Text="Дисконтная карта: 00002345"/>
	   		<Barcode BarcodeType="EAN13" Barcode="2000021262157"/>
	   	</Positions>
	   </Document>
	*/
	type Barcode struct {
		Barcodetype string `xml:"BarcodeType,attr" binding:"required"`
		Barcode     string `xml:"Barcode,attr" binding:"required"`
	}
	type TextString struct {
		Text string `xml:"Text,attr" binding:"required"`
	}
	type Positions struct {
		TextStr []TextString `xml:"TextString"`
		Barcode `xml:"Barcode"`
	}
	type Document struct {
		XMLName   xml.Name `xml:"Document"`
		Positions `xml:"Positions"`
	}
	var inp = Document{}
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

	if err = c.ShouldBindXML(&inp); err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	errcode, err := kkm.FNGetStatus()
	if err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errcode == 0 {
		fnstate := kkm.FNGetFNState()
		ShiftState := int(fnstate.FNSessionState + 1) //1 - Закрыта 2 - Открыта 3 - Истекла
		if ShiftState != 2 {
			if ShiftState == 3 {
				c.XML(http.StatusBadRequest, gin.H{"error": "Смена истекла, необходимо закрытие"})
			} else {
				c.XML(http.StatusBadRequest, gin.H{"error": "Смена закрыта"})
			}
			return
		}
	} else {
		c.XML(http.StatusBadRequest, gin.H{"error": kkm.ParseErrState(errcode)})
		return
	}
	admpass := kkm.GetAdminPass()
	for i := 0; i < len(inp.TextStr); i++ {
		tx := inp.TextStr[i].Text
		errcode, _ = kkm.PrintString(admpass, tx)
		if errcode > 0 {
			c.XML(http.StatusBadRequest, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
			return
		}
	}
	barcode := inp.Barcode.Barcode
	bartype := inp.Barcode.Barcodetype
	decodedbarcode, err := base64.StdEncoding.DecodeString(barcode)
	if err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": true, "message": err.Error()})
		return
	}
	errcode, err = kkm.PrintBarCode(admpass, bartype, decodedbarcode)
	if err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": true, "message": err.Error()})
		return
	}
	if err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}
	c.XML(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
}
